// Copyright (c) 2026 the packer-plugin-cloud-evolution authors.
// SPDX-License-Identifier: Apache-2.0

package evolution

import (
	"context"
	"errors"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	maxHTTPAttempts = 4
	backoffBase     = 200 * time.Millisecond
	backoffCap      = 2 * time.Second
	maxRetryAfter   = 10 * time.Second
)

func idempotent(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodDelete, http.MethodOptions:
		return true
	default:
		return false
	}
}

// shouldRetry decides whether another attempt is safe.
//
// Evolution has no client-token / idempotency key. POST /vms and POST /images
// must not be retried on timeout or 5xx — the object may already exist.
// 429 is retried for every method: the request was not accepted.
func shouldRetry(method string, err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return idempotent(method)
	}
	if api, ok := AsAPIError(err); ok {
		if api.Unauthorized() {
			return false // 401 is a single in-request token refresh, not this loop
		}
		if api.Status == http.StatusTooManyRequests {
			return true
		}
		return api.Retryable() && idempotent(method)
	}
	var re *RequestError
	if errors.As(err, &re) {
		if re.Timeout || re.Temporary {
			return idempotent(method)
		}
	}
	var netErr net.Error
	if errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary()) {
		return idempotent(method)
	}
	return false
}

func retryAfter(hdr http.Header) time.Duration {
	if hdr == nil {
		return 0
	}
	raw := strings.TrimSpace(hdr.Get("Retry-After"))
	if raw == "" {
		return 0
	}
	if sec, err := strconv.Atoi(raw); err == nil && sec > 0 {
		d := time.Duration(sec) * time.Second
		if d > maxRetryAfter {
			return maxRetryAfter
		}
		return d
	}
	if when, err := http.ParseTime(raw); err == nil {
		d := time.Until(when)
		if d < 0 {
			return 0
		}
		if d > maxRetryAfter {
			return maxRetryAfter
		}
		return d
	}
	return 0
}

func backoff(attempt int, retryAfter time.Duration) time.Duration {
	if retryAfter > 0 {
		return retryAfter
	}
	if attempt < 0 {
		attempt = 0
	}
	d := backoffBase << attempt
	if d > backoffCap {
		d = backoffCap
	}
	// Full jitter, AWS style: [0, d].
	if d <= 0 {
		return backoffBase
	}
	return time.Duration(rand.Int63n(int64(d) + 1))
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
