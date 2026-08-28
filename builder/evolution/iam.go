// Copyright (c) 2026 the packer-plugin-cloud-evolution authors.
// SPDX-License-Identifier: Apache-2.0

package evolution

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	defaultIAMURL    = "https://iam.api.cloud.ru"
	iamTokenPath     = "/api/v1/auth/token"
	defaultTokenTTL  = time.Hour
	tokenRefreshSkew = 5 * time.Minute
)

// TokenSource yields a Bearer token. IAM keys expire in about an hour;
// the HTTP client retries once on 401 after Invalidate.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
	Invalidate()
}

type staticToken string

func (s staticToken) Token(context.Context) (string, error) { return string(s), nil }
func (s staticToken) Invalidate()                           {}

type iamKeySource struct {
	url       string
	keyID     string
	secret    string
	http      *http.Client
	userAgent string
	mu        sync.Mutex
	token     string
	expiry    time.Time
}

type iamTokenRequest struct {
	KeyID  string `json:"keyId"`
	Secret string `json:"secret"`
}

// Live IAM (2026-08-21) returns OIDC-shaped JSON: access_token, expires_in=3600.
// CamelCase aliases stay for proto-JSON variants.
type iamTokenResponse struct {
	AccessToken      string `json:"access_token"`
	AccessTokenCamel string `json:"accessToken"`
	Token            string `json:"token"`
	ExpiresIn        int    `json:"expires_in"`
	ExpiresInCamel   int    `json:"expiresIn"`
	ExpiresAt        string `json:"expires_at"`
	ExpiresAtCamel   string `json:"expiresAt"`
}

func (r iamTokenResponse) accessToken() string {
	for _, v := range []string{r.AccessToken, r.AccessTokenCamel, r.Token} {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func (r iamTokenResponse) ttl() time.Duration {
	sec := r.ExpiresIn
	if sec == 0 {
		sec = r.ExpiresInCamel
	}
	if sec > 0 {
		return time.Duration(sec) * time.Second
	}
	for _, raw := range []string{r.ExpiresAt, r.ExpiresAtCamel} {
		if raw == "" {
			continue
		}
		if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
			if remaining := time.Until(parsed); remaining > 0 {
				return remaining
			}
		}
	}
	return defaultTokenTTL
}

func newIAMKeySource(iamURL, keyID, secret string, httpClient *http.Client, userAgent string) *iamKeySource {
	base := strings.TrimRight(strings.TrimSpace(iamURL), "/")
	if base == "" {
		base = defaultIAMURL
	}
	if userAgent == "" {
		userAgent = defaultUserAgent()
	}
	return &iamKeySource{
		url:       base + iamTokenPath,
		keyID:     keyID,
		secret:    secret,
		http:      httpClient,
		userAgent: userAgent,
	}
}

func (s *iamKeySource) Token(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token != "" && time.Now().Add(tokenRefreshSkew).Before(s.expiry) {
		return s.token, nil
	}
	token, expiry, err := s.fetch(ctx)
	if err != nil {
		return "", err
	}
	s.token = token
	s.expiry = expiry
	return token, nil
}

func (s *iamKeySource) Invalidate() {
	s.mu.Lock()
	s.token = ""
	s.expiry = time.Time{}
	s.mu.Unlock()
}

func (s *iamKeySource) fetch(ctx context.Context) (string, time.Time, error) {
	body, err := json.Marshal(iamTokenRequest{KeyID: s.keyID, Secret: s.secret})
	if err != nil {
		return "", time.Time{}, err
	}
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			if err := sleepCtx(ctx, backoff(attempt-1, 0)); err != nil {
				return "", time.Time{}, err
			}
		}
		token, expiry, err := s.fetchOnce(ctx, body)
		if err == nil {
			return token, expiry, nil
		}
		last = err
		if api, ok := AsAPIError(err); ok && api.Retryable() {
			continue
		}
		// Token issuance has no side effects worth protecting; transient
		// transport failures (dial, TLS, timeout) are safe to retry.
		var re *RequestError
		if errors.As(err, &re) && (re.Timeout || re.Temporary) {
			continue
		}
		return "", time.Time{}, err
	}
	return "", time.Time{}, last
}

func (s *iamKeySource) fetchOnce(ctx context.Context, body []byte) (string, time.Time, error) {
	ctx, cancel := ensureDeadline(ctx, httpClientTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(body))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("iam token: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", s.userAgent)
	resp, err := s.http.Do(req)
	if err != nil {
		return "", time.Time{}, classifyTransport(http.MethodPost, iamTokenPath, err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 32<<10))
		_ = resp.Body.Close()
	}()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody+1))
	if err != nil {
		return "", time.Time{}, classifyTransport(http.MethodPost, iamTokenPath, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", time.Time{}, parseAPIErrorMeta(resp.StatusCode, payload, http.MethodPost, iamTokenPath, responseRequestID(resp.Header))
	}
	var view iamTokenResponse
	if err := json.Unmarshal(payload, &view); err != nil {
		return "", time.Time{}, fmt.Errorf("iam token: decode: %w", err)
	}
	token := view.accessToken()
	if token == "" {
		return "", time.Time{}, fmt.Errorf("iam token: response did not include access_token")
	}
	return token, time.Now().Add(view.ttl()), nil
}
