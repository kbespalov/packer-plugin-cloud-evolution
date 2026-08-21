// Copyright (c) 2026 the packer-plugin-cloud-evolution authors.
// SPDX-License-Identifier: Apache-2.0

package evolution

import (
	"context"
	"errors"
	"fmt"
	"time"
)

func poll(ctx context.Context, interval, timeout time.Duration, fn func(context.Context) (bool, error)) error {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	if timeout <= 0 {
		timeout = 15 * time.Minute
	}
	if interval > timeout {
		interval = timeout
	}
	deadline := time.Now().Add(timeout)
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	var lastErr error
	for {
		if err := ctx.Err(); err != nil {
			return pollTimeout(timeout, lastErr, err)
		}
		ok, err := fn(ctx)
		if err != nil {
			lastErr = err
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return pollTimeout(timeout, lastErr, err)
			}
			if api, yes := AsAPIError(err); yes && api.Retryable() {
				ok = false
			} else {
				return err
			}
		}
		if ok {
			return nil
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return pollTimeout(timeout, lastErr, context.DeadlineExceeded)
		}
		sleep := interval
		if sleep > remaining {
			sleep = remaining
		}
		if err := sleepCtx(ctx, sleep); err != nil {
			return pollTimeout(timeout, lastErr, err)
		}
	}
}

func pollTimeout(timeout time.Duration, last, cause error) error {
	if last != nil && !errors.Is(last, cause) {
		return fmt.Errorf("timeout after %s (last: %w)", timeout, last)
	}
	if cause != nil && !errors.Is(cause, context.DeadlineExceeded) && !errors.Is(cause, context.Canceled) {
		return fmt.Errorf("timeout after %s: %w", timeout, cause)
	}
	return fmt.Errorf("timeout after %s", timeout)
}

// ensureDeadline gives ctx a deadline when the caller did not. HTTP and
// cleanup must not inherit a bare Background that can block forever.
func ensureDeadline(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if d <= 0 {
		d = httpClientTimeout
	}
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, d)
}
