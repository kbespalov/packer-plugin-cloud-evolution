// Copyright (c) 2026 the packer-plugin-cloud-evolution authors.
// SPDX-License-Identifier: Apache-2.0

package evolution

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"
)

const maxErrorBody = 512

// APIError is a Cloud.ru Evolution Compute / IAM error.
type APIError struct {
	Status    int
	Code      string
	Message   string
	Field     string
	Method    string
	Path      string
	RequestID string
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "evolution %s %s http %d", emptyDash(e.Method), emptyDash(e.Path), e.Status)
	if e.Code != "" {
		fmt.Fprintf(&b, " %s", e.Code)
	}
	if e.Field != "" {
		fmt.Fprintf(&b, " field=%s", e.Field)
	}
	if e.RequestID != "" {
		fmt.Fprintf(&b, " req_id=%s", e.RequestID)
	}
	if e.Message != "" {
		fmt.Fprintf(&b, ": %s", e.Message)
	}
	return b.String()
}

func emptyDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func (e *APIError) NotFound() bool { return e != nil && e.Status == http.StatusNotFound }

func (e *APIError) Retryable() bool {
	if e == nil {
		return false
	}
	switch e.Status {
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return e.Status >= 500
}

func (e *APIError) Unauthorized() bool {
	return e != nil && e.Status == http.StatusUnauthorized
}

func (e *APIError) Quota() bool {
	if e == nil {
		return false
	}
	blob := strings.ToLower(e.Code + " " + e.Message)
	return strings.Contains(blob, "quota")
}

func AsAPIError(err error) (*APIError, bool) {
	var api *APIError
	if errors.As(err, &api) {
		return api, true
	}
	return nil, false
}

// RequestError is a transport failure (dial, TLS, timeout) before a status.
type RequestError struct {
	Method    string
	Path      string
	Err       error
	Timeout   bool
	Temporary bool
}

func (e *RequestError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("evolution %s %s: %v", emptyDash(e.Method), emptyDash(e.Path), e.Err)
}

func (e *RequestError) Unwrap() error { return e.Err }

type apiErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field"`
}

func parseAPIError(status int, payload []byte) error {
	return parseAPIErrorMeta(status, payload, "", "", "")
}

func parseAPIErrorMeta(status int, payload []byte, method, path, reqID string) error {
	trimmed := strings.TrimSpace(string(payload))
	out := &APIError{Status: status, Method: method, Path: path, RequestID: reqID}
	if trimmed == "" {
		out.Message = http.StatusText(status)
		return out
	}
	var one apiErrorBody
	if json.Unmarshal([]byte(trimmed), &one) == nil && (one.Code != "" || one.Message != "") {
		out.Code, out.Message, out.Field = one.Code, clipErrorText(one.Message), one.Field
		return out
	}
	var many []apiErrorBody
	if json.Unmarshal([]byte(trimmed), &many) == nil && len(many) > 0 {
		first := many[0]
		out.Code, out.Message, out.Field = first.Code, clipErrorText(first.Message), first.Field
		return out
	}
	out.Message = clipErrorText(trimmed)
	return out
}

func clipErrorText(s string) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= maxErrorBody {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxErrorBody]) + "…"
}

func responseRequestID(h http.Header) string {
	if h == nil {
		return ""
	}
	for _, key := range []string{"X-Request-Id", "X-Request-ID", "Request-Id"} {
		if v := strings.TrimSpace(h.Get(key)); v != "" {
			return v
		}
	}
	return ""
}
