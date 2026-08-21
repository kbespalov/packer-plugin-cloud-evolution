// Copyright (c) 2026 the packer-plugin-cloud-evolution authors.
// SPDX-License-Identifier: Apache-2.0

package evolution

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestShouldRetry(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		method string
		err    error
		want   bool
	}{
		{"nil", http.MethodGet, nil, false},
		{"canceled", http.MethodGet, context.Canceled, false},
		{"get 503", http.MethodGet, &APIError{Status: 503, Code: "busy"}, true},
		{"get 429", http.MethodGet, &APIError{Status: 429, Code: "rate"}, true},
		{"post 429", http.MethodPost, &APIError{Status: 429, Code: "rate"}, true},
		{"post 500", http.MethodPost, &APIError{Status: 500, Code: "boom"}, false},
		{"post timeout", http.MethodPost, &RequestError{Timeout: true, Err: errors.New("timeout")}, false},
		{"get timeout", http.MethodGet, &RequestError{Timeout: true, Err: errors.New("timeout")}, true},
		{"delete 502", http.MethodDelete, &APIError{Status: 502}, true},
		{"get 422", http.MethodGet, &APIError{Status: 422, Code: "extra_forbidden"}, false},
		{"get 401", http.MethodGet, &APIError{Status: 401}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := shouldRetry(tc.method, tc.err); got != tc.want {
				t.Fatalf("shouldRetry(%s, %v)=%v want %v", tc.method, tc.err, got, tc.want)
			}
		})
	}
}

func TestRetryAfterSeconds(t *testing.T) {
	t.Parallel()
	h := http.Header{}
	h.Set("Retry-After", "2")
	if d := retryAfter(h); d != 2*time.Second {
		t.Fatalf("%s", d)
	}
	h.Set("Retry-After", "999")
	if d := retryAfter(h); d != maxRetryAfter {
		t.Fatalf("capped %s", d)
	}
}

func TestBackoffHonorsRetryAfter(t *testing.T) {
	t.Parallel()
	if d := backoff(0, time.Second); d != time.Second {
		t.Fatalf("%s", d)
	}
}

func TestAPIErrorQuota(t *testing.T) {
	t.Parallel()
	err := parseAPIError(422, []byte(`{"code":"organization_quota_exceeded","message":"Organization quota for compute.image.custome exceeded"}`))
	api, ok := AsAPIError(err)
	if !ok || !api.Quota() {
		t.Fatalf("%v", err)
	}
}

func TestClipErrorText(t *testing.T) {
	t.Parallel()
	long := make([]byte, 2000)
	for i := range long {
		long[i] = 'x'
	}
	err := parseAPIError(500, long)
	if len(err.Error()) < 100 || !containsRune(err.Error(), '…') {
		t.Fatalf("expected clipped body: %s", err.Error())
	}
}

func containsRune(s string, r rune) bool {
	for _, c := range s {
		if c == r {
			return true
		}
	}
	return false
}
