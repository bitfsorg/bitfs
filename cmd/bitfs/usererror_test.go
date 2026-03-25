package main

import (
	"errors"
	"testing"
)

func TestUserMessage(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: "",
		},
		{
			name:     "simple error",
			err:      errors.New("file not found"),
			expected: "file not found",
		},
		{
			name:     "deeply nested vault error",
			err:      errors.New("vault: ensure root: vault: no fee UTXO with >= 2000 sats; run 'bitfs fund' first: insufficient funds"),
			expected: "vault: no fee UTXO with >= 2000 sats; run 'bitfs fund' first: insufficient funds",
		},
		{
			name:     "double vault prefix",
			err:      errors.New("vault: vault: something went wrong"),
			expected: "vault: something went wrong",
		},
		{
			name:     "double engine prefix",
			err:      errors.New("engine: engine: lock failed"),
			expected: "engine: lock failed",
		},
		{
			name:     "normal error passthrough",
			err:      errors.New("wallet: seed decryption failed (wrong password or corrupted data)"),
			expected: "wallet: seed decryption failed (wrong password or corrupted data)",
		},
		{
			name:     "deep network error chain",
			err:      errors.New("network: get tx status: woc: get tx status: network: transaction not found: /tx/abc123"),
			expected: "transaction not found: abc123",
		},
		{
			name:     "network error generic",
			err:      errors.New("network: get headers: network: connection refused"),
			expected: "connection refused",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := userMessage(tt.err)
			if got != tt.expected {
				t.Errorf("userMessage() = %q, want %q", got, tt.expected)
			}
		})
	}
}
