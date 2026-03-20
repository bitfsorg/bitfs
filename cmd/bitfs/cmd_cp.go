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

// runCp handles the "bitfs cp" command.
func runCp(args []string) int {
	fs := flag.NewFlagSet("cp", flag.ContinueOnError)
	vaultName := fs.String("vault", "", "vault name")
	jsonOut := fs.Bool("json", false, "JSON output")
	dataDir := fs.String("datadir", config.DefaultDataDir(), "data directory")
	password := fs.String("password", "", "wallet password (for testing)")

	if err := fs.Parse(args); err != nil {
		return exitUsageError
	}

	if fs.NArg() < 2 {
		fmt.Fprintf(os.Stderr, "Usage: bitfs cp <src> <dst> [--vault N] [--json]\n")
		return exitUsageError
	}

	src := fs.Arg(0)
	dst := fs.Arg(1)

	pass, err := resolvePassword(*password)
	if err != nil {
		if *jsonOut {
			return writeJSONErr("cp", exitWalletError, err)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return exitWalletError
	}

	eng, err := vault.New(*dataDir, pass)
	if err != nil {
		if *jsonOut {
			return writeJSONErr("cp", exitWalletError, err)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return exitWalletError
	}
	defer func() { _ = eng.Close() }()

	// Configure blockchain for auto-broadcast.
	cfg, cfgErr := config.LoadConfig(config.ConfigPath(*dataDir))
	if cfgErr != nil {
		cfg = config.DefaultConfig()
	}
	if cfg.Network != "regtest" { configureChain(eng, "", "", "", cfg.Network) }

	vaultIdx, err := eng.ResolveVaultIndex(*vaultName)
	if err != nil {
		if *jsonOut {
			return writeJSONErr("cp", exitNotFound, err)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return exitNotFound
	}

	result, err := eng.Copy(&vault.CopyOpts{
		VaultIndex: vaultIdx,
		SrcPath:    src,
		DstPath:    dst,
	})
	if err != nil {
		if *jsonOut {
			return writeJSONErr("cp", exitError, err)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return exitError
	}


	if *jsonOut {
		return writeJSONResult(&cmdResult{
			OK:      true,
			Command: "cp",
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
