//go:build e2e

package testutil

import "time"

// NetworkAwareTestTimeout returns a timeout suitable for the active network.
// On regtest it keeps the provided default timeout. On live networks it ensures
// at least confirm-timeout + a small buffer so tests don't fail early.
func NetworkAwareTestTimeout(defaultTimeout time.Duration) time.Duration {
	cfg := LoadConfig()
	if cfg.IsRegtest() {
		return defaultTimeout
	}

	minTimeout := cfg.ConfirmTimeout + 5*time.Minute
	if minTimeout > defaultTimeout {
		return minTimeout
	}
	return defaultTimeout
}
