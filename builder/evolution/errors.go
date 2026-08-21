// Copyright (c) 2026 the packer-plugin-cloud-evolution authors.
// SPDX-License-Identifier: Apache-2.0

package evolution

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
)

const (
	defaultCleanupTimeout  = 10 * time.Minute
	artifactDestroyTimeout = 2 * time.Minute
)

func isNotFound(err error) bool {
	api, ok := AsAPIError(err)
	return ok && api.NotFound()
}

// lookupExists maps a Get/Find result to (exists, err). A 404 is "does not
// exist", not a hard failure. Any other error is returned so the caller does
// not treat a timeout as "name is free".
func lookupExists(id string, err error) (bool, error) {
	if err == nil {
		return id != "", nil
	}
	if isNotFound(err) {
		return false, nil
	}
	return false, err
}

func alreadyExists(err error) bool {
	api, ok := AsAPIError(err)
	if !ok {
		return false
	}
	blob := strings.ToLower(api.Code + " " + api.Message)
	for _, n := range []string{"already_exists", "already exists", "duplicate"} {
		if strings.Contains(blob, n) {
			return true
		}
	}
	return api.Status == http.StatusConflict && strings.Contains(blob, "name")
}

func annotateQuota(err error) error {
	if err == nil {
		return nil
	}
	if api, ok := AsAPIError(err); ok && api.Quota() {
		return fmt.Errorf("%w (delete an unused private image or floating IP; Evolution org quota is small)", err)
	}
	return err
}

func detachAlreadyDone(err error) bool {
	if err == nil {
		return true
	}
	api, ok := AsAPIError(err)
	if !ok {
		return false
	}
	blob := strings.ToLower(api.Code + " " + api.Message)
	for _, n := range []string{"not_attached", "already_detached", "is_not_attached", "not attached"} {
		if strings.Contains(blob, n) {
			return true
		}
	}
	return false
}

func resourceBusy(err error) bool {
	api, ok := AsAPIError(err)
	if !ok {
		return false
	}
	if api.Retryable() {
		return true
	}
	if api.Status != http.StatusConflict && api.Status != http.StatusUnprocessableEntity {
		return false
	}
	blob := strings.ToLower(api.Code + " " + api.Message)
	for _, n := range []string{"in_use", "in-use", "busy", "attached", "locked", "current_state", "in current state"} {
		if strings.Contains(blob, n) {
			return true
		}
	}
	return false
}

func cleanupContext(state multistep.StateBag) (context.Context, context.CancelFunc) {
	timeout := defaultCleanupTimeout
	if raw, ok := state.GetOk("config"); ok {
		if cfg, ok := raw.(*Config); ok && cfg != nil && cfg.StateTimeout > 0 && cfg.StateTimeout < timeout {
			timeout = cfg.StateTimeout
		}
	}
	return context.WithTimeout(context.Background(), timeout)
}
