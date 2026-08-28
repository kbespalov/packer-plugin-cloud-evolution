// Copyright (c) 2026 the packer-plugin-cloud-evolution authors.
// SPDX-License-Identifier: Apache-2.0

package evolution

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/kbespalov/packer-plugin-cloud-evolution/version"
)

const (
	defaultComputeURL = "https://compute.api.cloud.ru"
	maxResponseBody   = 1 << 20
	httpClientTimeout = 30 * time.Second
	dialTimeout       = 10 * time.Second
	tlsHandshakeTO    = 10 * time.Second
	responseHeaderTO  = 20 * time.Second
	idleConnTimeout   = 90 * time.Second
)

// Client is a thin REST client for Evolution Compute. There is no official
// Go SDK; wire format follows the published OpenAPI (SVC Public API).
//
// Retry policy (no idempotency key on this API):
//   - GET/DELETE: 429 / 502 / 503 / 504 and transient net errors
//   - POST: 429 only, plus a single 401 after IAM refresh
//   - POST timeout / 5xx is not retried (the object may already exist);
//     CreateInstance instead recovers by looking the VM up by name
type Client struct {
	baseURL    string
	projectID  string
	tokens     TokenSource
	httpClient *http.Client
	userAgent  string
	// Zero means the createRecover* defaults; tests shrink these.
	recoverInterval time.Duration
	recoverTimeout  time.Duration
}

// ClientConfig configures Client.
type ClientConfig struct {
	BaseURL    string
	IAMURL     string
	ProjectID  string
	Token      string
	KeyID      string
	KeySecret  string
	HTTPClient *http.Client
	UserAgent  string
}

func NewClient(cfg ClientConfig) (*Client, error) {
	if strings.TrimSpace(cfg.ProjectID) == "" {
		return nil, fmt.Errorf("project_id is required")
	}
	httpClient := ensureHTTPClient(cfg.HTTPClient)
	base, err := normalizeBaseURL(cfg.BaseURL, defaultComputeURL)
	if err != nil {
		return nil, fmt.Errorf("compute_url: %w", err)
	}
	iamURL := cfg.IAMURL
	if strings.TrimSpace(iamURL) != "" {
		iamURL, err = normalizeBaseURL(iamURL, defaultIAMURL)
		if err != nil {
			return nil, fmt.Errorf("iam_url: %w", err)
		}
	}
	ua := strings.TrimSpace(cfg.UserAgent)
	if ua == "" {
		ua = defaultUserAgent()
	}
	var tokens TokenSource
	switch {
	case strings.TrimSpace(cfg.KeyID) != "" && strings.TrimSpace(cfg.KeySecret) != "":
		tokens = newIAMKeySource(iamURL, cfg.KeyID, cfg.KeySecret, httpClient, ua)
	case strings.TrimSpace(cfg.Token) != "":
		tokens = staticToken(strings.TrimSpace(cfg.Token))
	default:
		return nil, fmt.Errorf("set key_id+key_secret or token")
	}
	return &Client{
		baseURL:    base,
		projectID:  cfg.ProjectID,
		tokens:     tokens,
		httpClient: httpClient,
		userAgent:  ua,
	}, nil
}

func defaultUserAgent() string {
	v := version.Version
	if version.VersionPrerelease != "" {
		v += "-" + version.VersionPrerelease
	}
	return "packer-plugin-cloud-evolution/" + v
}

func ensureHTTPClient(c *http.Client) *http.Client {
	if c == nil {
		return newAPIHTTPClient()
	}
	if c.Timeout > 0 {
		return c
	}
	clone := *c
	clone.Timeout = httpClientTimeout
	return &clone
}

func newAPIHTTPClient() *http.Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = http.ProxyFromEnvironment
	t.DialContext = (&net.Dialer{Timeout: dialTimeout, KeepAlive: 30 * time.Second}).DialContext
	t.TLSHandshakeTimeout = tlsHandshakeTO
	t.ResponseHeaderTimeout = responseHeaderTO
	t.ExpectContinueTimeout = 1 * time.Second
	t.IdleConnTimeout = idleConnTimeout
	t.MaxIdleConns = 32
	t.MaxIdleConnsPerHost = 8
	t.ForceAttemptHTTP2 = true
	return &http.Client{
		Transport: t,
		Timeout:   httpClientTimeout,
		// Do not follow redirects: Authorization would be replayed to another host.
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func normalizeBaseURL(raw, fallback string) (string, error) {
	s := strings.TrimRight(strings.TrimSpace(raw), "/")
	if s == "" {
		s = fallback
	}
	u, err := url.Parse(s)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("not a valid URL")
	}
	switch u.Scheme {
	case "http", "https":
	default:
		return "", fmt.Errorf("must be http or https")
	}
	return strings.TrimRight(u.String(), "/"), nil
}

func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	if ctx == nil {
		ctx = context.Background()
	}
	var last error
	var retryHint time.Duration
	for attempt := 0; attempt < maxHTTPAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			if last != nil {
				return last
			}
			return err
		}
		if attempt > 0 {
			if err := sleepCtx(ctx, backoff(attempt-1, retryHint)); err != nil {
				if last != nil {
					return last
				}
				return err
			}
		}
		retryHint = 0
		err := c.doOnce(ctx, method, path, in, out, false)
		if err == nil {
			return nil
		}
		last = err
		if hint := retryAfterFrom(err); hint > 0 {
			retryHint = hint
		}
		if ctx.Err() != nil || !shouldRetry(method, err) {
			return err
		}
		log.Printf("[DEBUG] evolution retry %s %s attempt=%d err=%s", method, path, attempt+1, err)
	}
	return last
}

func (c *Client) doOnce(ctx context.Context, method, path string, in, out any, authRetried bool) error {
	reqCtx, cancel := ensureDeadline(ctx, httpClientTimeout)
	defer cancel()
	var payload []byte
	if in != nil {
		var err error
		payload, err = json.Marshal(in)
		if err != nil {
			return fmt.Errorf("marshal %s %s: %w", method, path, err)
		}
	}
	reqURL, err := joinURL(c.baseURL, path)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(reqCtx, method, reqURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	if payload != nil {
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(payload)), nil
		}
		req.ContentLength = int64(len(payload))
		req.Header.Set("Content-Type", "application/json")
	}
	token, err := c.tokens.Token(reqCtx)
	if err != nil {
		return err
	}
	reqID := newRequestID()
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("X-Request-ID", reqID)

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return classifyTransport(method, path, err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 32<<10))
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody+1))
	if err != nil {
		return &RequestError{Method: method, Path: path, Err: err, Temporary: true}
	}
	if len(body) > maxResponseBody {
		return fmt.Errorf("evolution %s %s: response larger than %d bytes", method, path, maxResponseBody)
	}

	rid := responseRequestID(resp.Header)
	if rid == "" {
		rid = reqID
	}
	log.Printf("[DEBUG] evolution %s %s status=%d dur=%s req_id=%s", method, path, resp.StatusCode, time.Since(start).Round(time.Millisecond), rid)

	if resp.StatusCode == http.StatusUnauthorized && !authRetried {
		c.tokens.Invalidate()
		return c.doOnce(ctx, method, path, in, out, true)
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if out == nil || len(body) == 0 {
			return nil
		}
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("decode %s %s: %w", method, path, err)
		}
		return nil
	}
	apiErr := parseAPIErrorMeta(resp.StatusCode, body, method, path, rid)
	if wait := retryAfter(resp.Header); wait > 0 {
		if api, ok := apiErr.(*APIError); ok {
			return &retryAfterError{APIError: api, after: wait}
		}
	}
	return apiErr
}

type retryAfterError struct {
	*APIError
	after time.Duration
}

// Unwrap lets errors.As reach the APIError. Without it, a 429/503 with a
// Retry-After header would stop matching AsAPIError and never be retried.
func (e *retryAfterError) Unwrap() error { return e.APIError }

func retryAfterFrom(err error) time.Duration {
	var ra *retryAfterError
	if errors.As(err, &ra) {
		return ra.after
	}
	return 0
}

func classifyTransport(method, path string, err error) error {
	timeout := errors.Is(err, context.DeadlineExceeded)
	temporary := false
	var netErr net.Error
	if errors.As(err, &netErr) {
		timeout = timeout || netErr.Timeout()
		temporary = netErr.Temporary()
	}
	return &RequestError{Method: method, Path: path, Err: err, Timeout: timeout, Temporary: temporary || timeout}
}

func joinURL(base, path string) (string, error) {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return "", fmt.Errorf("refusing absolute request URL")
	}
	if i := strings.IndexByte(path, '?'); i >= 0 {
		root, err := url.JoinPath(base, path[:i])
		if err != nil {
			return "", err
		}
		return root + path[i:], nil
	}
	return url.JoinPath(base, path)
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
