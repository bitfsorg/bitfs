// Copyright (c) 2024 The BitFS developers
// Use of this source code is governed by the Open BSV License v5
// that can be found in the LICENSE file.

package buy

import (
	"errors"
	"fmt"
	"io"

	"github.com/bitfsorg/bitfs/internal/client"
)

// Standard exit codes (shared by bitfs and b-tools).
const (
	ExitSuccess     = 0
	ExitError       = 1
	ExitUsageError  = 2
	ExitWalletError = 3
	ExitNetError    = 4
	ExitPermError   = 5
	ExitNotFound    = 6
	ExitConflict    = 7
)

// ExitCodeFromError maps a client error to a CLI exit code.
// 0=success, 1=general, 2=usage, 4=network/timeout, 5=payment/perm, 6=not found.
func ExitCodeFromError(err error) int {
	switch {
	case errors.Is(err, client.ErrNotFound):
		return ExitNotFound
	case errors.Is(err, client.ErrTimeout), errors.Is(err, client.ErrNetwork), errors.Is(err, client.ErrServer):
		return ExitNetError
	case errors.Is(err, client.ErrPaymentRequired):
		return ExitPermError
	default:
		return ExitError
	}
}

// ErrorMessage returns a short human-readable message for a client error.
func ErrorMessage(err error) string {
	switch {
	case errors.Is(err, client.ErrNotFound):
		return "not found"
	case errors.Is(err, client.ErrTimeout):
		return "request timeout"
	case errors.Is(err, client.ErrNetwork):
		return "network error"
	case errors.Is(err, client.ErrServer):
		return "server error"
	default:
		return err.Error()
	}
}

// HandleError prints a formatted error to stderr and returns the exit code.
// The toolName is prefixed to the message (e.g. "bcat", "bget").
func HandleError(err error, toolName string, stderr io.Writer) int {
	code := ExitCodeFromError(err)
	msg := ErrorMessage(err)
	if code == 4 && !errors.Is(err, client.ErrTimeout) {
		// For network/server errors, include the underlying message.
		fmt.Fprintf(stderr, "%s: %s: %v\n", toolName, msg, err)
	} else {
		fmt.Fprintf(stderr, "%s: %s\n", toolName, msg)
	}
	return code
}
