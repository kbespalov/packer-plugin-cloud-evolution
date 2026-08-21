// Copyright (c) 2026 the packer-plugin-cloud-evolution authors.
// SPDX-License-Identifier: Apache-2.0

package evolution

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const defaultIAMURL = "https://iam.api.cloud.ru"

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
	iamURL    string
	keyID     string
	secret    string
	http      *http.Client
	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

func newIAMKeySource(iamURL, keyID, secret string, httpClient *http.Client) *iamKeySource {
	if strings.TrimRight(iamURL, "/") == "" {
		iamURL = defaultIAMURL
	}
	return &iamKeySource{iamURL: strings.TrimRight(iamURL, "/"), keyID: keyID, secret: secret, http: httpClient}
}

func (s *iamKeySource) Token(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token != "" && time.Now().Before(s.expiresAt) {
		return s.token, nil
	}
	body, err := json.Marshal(map[string]string{"keyId": s.keyID, "secret": s.secret})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.iamURL+"/api/v1/auth/token", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := s.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("iam token: %w", err)
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", parseAPIError(resp.StatusCode, payload)
	}
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(payload, &out); err != nil || out.AccessToken == "" {
		return "", fmt.Errorf("iam token: decode access_token: %w", err)
	}
	ttl := time.Duration(out.ExpiresIn) * time.Second
	if ttl <= 0 {
		ttl = time.Hour
	}
	// Refresh a minute early so a long image wait does not 401 mid-poll.
	if ttl > 2*time.Minute {
		ttl -= time.Minute
	}
	s.token = out.AccessToken
	s.expiresAt = time.Now().Add(ttl)
	return s.token, nil
}

func (s *iamKeySource) Invalidate() {
	s.mu.Lock()
	s.token = ""
	s.expiresAt = time.Time{}
	s.mu.Unlock()
}
