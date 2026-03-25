// Copyright (c) 2024 The BitFS developers
// Use of this source code is governed by the Open BSV License v5
// that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"os"

	"golang.org/x/term"

	"github.com/bitfsorg/bitfs/internal/engine"
	"github.com/bitfsorg/libbitfs-go/config"
)

// runVaultExport exports the vault root private key.
func runVaultExport(args []string) int {
	fs := flag.NewFlagSet("vault export", flag.ContinueOnError)
	dataDir := fs.String("datadir", config.DefaultDataDir(), "data directory")
	password := fs.String("password", "", "wallet password (for testing)")
	format := fs.String("format", "wif", "export format: wif, hex, or seed-path")
	yes := fs.Bool("yes", false, "skip confirmation prompt")
	network := addNetworkFlag(fs)

	if err := fs.Parse(args); err != nil {
		return exitUsageError
	}
	if !resolveNetworkDataDir(fs, network, dataDir) {
		return exitUsageError
	}

	if fs.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "Usage: bitfs vault export <name> [--format wif|hex|seed-path] [--yes]\n")
		return exitUsageError
	}

	vaultName := fs.Arg(0)

	// Load wallet first to verify vault exists before showing warning.
	pass, err := resolvePassword(*password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", userMessage(err))
		return exitWalletError
	}

	eng := engine.New(*dataDir, pass)

	// Pre-check: verify the vault exists before prompting (use wif to avoid format errors).
	if _, checkErr := eng.VaultExport(vaultName, "wif"); checkErr != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", userMessage(checkErr))
		return exitNotFound
	}

	// For wif/hex formats, warn about private key exposure.
	if *format == "wif" || *format == "hex" {
		if !*yes {
			if !term.IsTerminal(int(os.Stdin.Fd())) {
				fmt.Fprintf(os.Stderr, "Error: refusing to export private key in non-interactive mode; use --yes to confirm\n")
				return exitUsageError
			}
			fmt.Fprintf(os.Stderr, "WARNING: This will display the vault root PRIVATE KEY.\n")
			fmt.Fprintf(os.Stderr, "Anyone with this key can control all files in vault %q.\n", vaultName)
			fmt.Fprintf(os.Stderr, "Continue? [y/N] ")

			var answer string
			fmt.Scanln(&answer)
			if answer != "y" && answer != "Y" {
				fmt.Fprintf(os.Stderr, "Aborted.\n")
				return exitSuccess
			}
		}
	}

	result, err := eng.VaultExport(vaultName, *format)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", userMessage(err))
		return exitError
	}

	fmt.Println(result)
	return exitSuccess
}
