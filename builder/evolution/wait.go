// Copyright (c) 2026 the packer-plugin-cloud-evolution authors.
// SPDX-License-Identifier: Apache-2.0

package evolution

import (
	"context"
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
	deadline := time.Now().Add(timeout)
	for {
		ok, err := fn(ctx)
		if err != nil {
			if api, yes := AsAPIError(err); yes && api.Retryable() {
				ok = false
			} else {
				return err
			}
		}
		if ok {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout after %s", timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}
