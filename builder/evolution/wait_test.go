// Copyright (c) 2026 the packer-plugin-cloud-evolution authors.
// SPDX-License-Identifier: Apache-2.0

package evolution

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestPollSucceeds(t *testing.T) {
	t.Parallel()
	n := 0
	err := poll(context.Background(), time.Millisecond, time.Second, func(context.Context) (bool, error) {
		n++
		return n >= 2, nil
	})
	if err != nil || n < 2 {
		t.Fatalf("n=%d err=%v", n, err)
	}
}

func TestPollTimeout(t *testing.T) {
	t.Parallel()
	err := poll(context.Background(), time.Millisecond, 8*time.Millisecond, func(context.Context) (bool, error) {
		return false, nil
	})
	if err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("%v", err)
	}
}

func TestPollRetryableThenOK(t *testing.T) {
	t.Parallel()
	n := 0
	err := poll(context.Background(), time.Millisecond, time.Second, func(context.Context) (bool, error) {
		n++
		if n == 1 {
			return false, &APIError{Status: http.StatusServiceUnavailable, Code: "busy"}
		}
		return true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPollCapsSleepToDeadline(t *testing.T) {
	t.Parallel()
	start := time.Now()
	err := poll(context.Background(), time.Hour, 40*time.Millisecond, func(context.Context) (bool, error) {
		return false, nil
	})
	if err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("%v", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("poll slept past its deadline: %s", elapsed)
	}
}

func TestPollCanceledContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	err := poll(ctx, time.Second, time.Minute, func(context.Context) (bool, error) {
		called = true
		return false, nil
	})
	if called {
		t.Fatal("must not call fn after ctx is already done")
	}
	if err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("%v", err)
	}
}

func TestPollFatalError(t *testing.T) {
	t.Parallel()
	want := errors.New("boom")
	err := poll(context.Background(), time.Millisecond, time.Second, func(context.Context) (bool, error) {
		return false, want
	})
	if !errors.Is(err, want) {
		t.Fatalf("%v", err)
	}
}
