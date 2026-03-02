// Copyright (c) 2024 The BitFS developers
// Use of this source code is governed by the Open BSV License v5
// that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/bitfsorg/libbitfs-go/config"
	"github.com/bitfsorg/libbitfs-go/vault"
)

// runMkdir handles the "bitfs mkdir" command.
func runMkdir(args []string) int {
	fs := flag.NewFlagSet("mkdir", flag.ContinueOnError)
	vaultName := fs.String("vault", "", "vault name")
	jsonOut := fs.Bool("json", false, "JSON output")
	dataDir := fs.String("datadir", config.DefaultDataDir(), "data directory")
	password := fs.String("password", "", "wallet password (for testing)")

	if err := fs.Parse(args); err != nil {
		return exitUsageError
	}

	if fs.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "Usage: bitfs mkdir <remote-path> [--vault N] [--json]\n")
		return exitUsageError
	}

	remotePath := fs.Arg(0)

	pass, err := resolvePassword(*password)
	if err != nil {
		if *jsonOut {
			return writeJSONErr("mkdir", exitWalletError, err)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return exitWalletError
	}

	eng, err := vault.New(*dataDir, pass)
	if err != nil {
		if *jsonOut {
			return writeJSONErr("mkdir", exitWalletError, err)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return exitWalletError
	}
	defer func() { _ = eng.Close() }()

	vaultIdx, err := eng.ResolveVaultIndex(*vaultName)
	if err != nil {
		if *jsonOut {
			return writeJSONErr("mkdir", exitNotFound, err)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return exitNotFound
	}

	result, err := eng.Mkdir(&vault.MkdirOpts{
		VaultIndex: vaultIdx,
		Path:       remotePath,
	})
	if err != nil {
		if *jsonOut {
			return writeJSONErr("mkdir", exitError, err)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return exitError
	}

	if *jsonOut {
		return writeJSONResult(&cmdResult{
			OK:      true,
			Command: "mkdir",
			Message: result.Message,
			TxID:    result.TxID,
			TxHex:   result.TxHex,
		})
	}

	fmt.Println(result.Message)
	if result.TxHex != "" {
		fmt.Printf("  TxID: %s\n", result.TxID)
		fmt.Printf("  Raw:  %s\n", result.TxHex)
	}

	return exitSuccess
}
