// Copyright (c) 2024 The BitFS developers
// Use of this source code is governed by the Open BSV License v5
// that can be found in the LICENSE file.

package main

import "strings"

// userMessage returns a clean, user-facing error message by trimming
// internal Go error chain prefixes that leak implementation details.
func userMessage(err error) string {
	if err == nil {
		return ""
	}

	msg := err.Error()

	// Collapse "vault: ensure root: vault: " → "vault: "
	msg = strings.Replace(msg, "vault: ensure root: vault: ", "vault: ", 1)

	// Remove repeated "vault: vault: " or "engine: engine: " prefixes.
	for strings.HasPrefix(msg, "vault: vault: ") {
		msg = msg[len("vault: "):]
	}
	for strings.HasPrefix(msg, "engine: engine: ") {
		msg = msg[len("engine: "):]
	}

	return msg
}
