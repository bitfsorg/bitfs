// Copyright (c) 2024 The BitFS developers
// Use of this source code is governed by the Open BSV License v5
// that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/bitfsorg/libbitfs-go/vault"
)

// printError writes an error to stderr. If the error wraps
// vault.ErrInsufficientFunds, it appends BSV purchase guidance.
func printError(err error) {
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	if errors.Is(err, vault.ErrInsufficientFunds) {
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "To add funds, run 'bitfs wallet fund' and send BSV to the displayed address.")
		fmt.Fprintln(os.Stderr, "Need BSV? Purchase from:")
		fmt.Fprintln(os.Stderr, "  ChangeNow  https://changenow.io/currencies/bitcoin-sv")
		fmt.Fprintln(os.Stderr, "  Changelly  https://changelly.com/exchange/bsv")
	}
}
