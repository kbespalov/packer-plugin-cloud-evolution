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
	"time"
)

const defaultComputeURL = "https://compute.api.cloud.ru"

// Client is a thin REST client for Evolution Compute. There is no official
// Go SDK; wire format follows the published OpenAPI (SVC Public API).
type Client struct {
	baseURL    string
	projectID  string
	tokens     TokenSource
	httpClient *http.Client
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
}

func NewClient(cfg ClientConfig) (*Client, error) {
	if strings.TrimSpace(cfg.ProjectID) == "" {
		return nil, fmt.Errorf("project_id is required")
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	base := strings.TrimRight(cfg.BaseURL, "/")
	if base == "" {
		base = defaultComputeURL
	}
	var tokens TokenSource
	switch {
	case strings.TrimSpace(cfg.KeyID) != "" && strings.TrimSpace(cfg.KeySecret) != "":
		tokens = newIAMKeySource(cfg.IAMURL, cfg.KeyID, cfg.KeySecret, httpClient)
	case strings.TrimSpace(cfg.Token) != "":
		tokens = staticToken(strings.TrimSpace(cfg.Token))
	default:
		return nil, fmt.Errorf("set key_id+key_secret or token")
	}
	return &Client{baseURL: base, projectID: cfg.ProjectID, tokens: tokens, httpClient: httpClient}, nil
}

func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	return c.doOnce(ctx, method, path, in, out, false)
}

func (c *Client) doOnce(ctx context.Context, method, path string, in, out any, retried bool) error {
	var body io.Reader
	if in != nil {
		payload, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("marshal %s %s: %w", method, path, err)
		}
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	token, err := c.tokens.Token(ctx)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusUnauthorized && !retried {
		c.tokens.Invalidate()
		return c.doOnce(ctx, method, path, in, out, true)
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if out == nil || len(payload) == 0 {
			return nil
		}
		if err := json.Unmarshal(payload, out); err != nil {
			return fmt.Errorf("decode %s %s: %w", method, path, err)
		}
		return nil
	}
	return parseAPIError(resp.StatusCode, payload)
}
