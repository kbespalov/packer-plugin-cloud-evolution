// Copyright (c) 2026 the packer-plugin-cloud-evolution authors.
// SPDX-License-Identifier: Apache-2.0

package evolution

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// APIError is a Cloud.ru Evolution Compute / IAM error.
type APIError struct {
	Status  int
	Code    string
	Message string
	Field   string
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "evolution http %d", e.Status)
	if e.Code != "" {
		fmt.Fprintf(&b, " %s", e.Code)
	}
	if e.Field != "" {
		fmt.Fprintf(&b, " field=%s", e.Field)
	}
	if e.Message != "" {
		fmt.Fprintf(&b, ": %s", e.Message)
	}
	return b.String()
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

func AsAPIError(err error) (*APIError, bool) {
	var api *APIError
	if errors.As(err, &api) {
		return api, true
	}
	return nil, false
}

type apiErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field"`
}

func parseAPIError(status int, payload []byte) error {
	trimmed := bytesTrim(payload)
	if len(trimmed) == 0 {
		return &APIError{Status: status, Message: http.StatusText(status)}
	}
	var one apiErrorBody
	if json.Unmarshal(trimmed, &one) == nil && (one.Code != "" || one.Message != "") {
		return &APIError{Status: status, Code: one.Code, Message: one.Message, Field: one.Field}
	}
	var many []apiErrorBody
	if json.Unmarshal(trimmed, &many) == nil && len(many) > 0 {
		first := many[0]
		return &APIError{Status: status, Code: first.Code, Message: first.Message, Field: first.Field}
	}
	return &APIError{Status: status, Message: string(trimmed)}
}

func bytesTrim(p []byte) []byte {
	return []byte(strings.TrimSpace(string(p)))
}
