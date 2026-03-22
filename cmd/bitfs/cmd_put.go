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

// runPut handles the "bitfs put" command.
func runPut(args []string) int {
	fs := flag.NewFlagSet("put", flag.ContinueOnError)
	vaultName := fs.String("vault", "", "vault name")
	access := fs.String("access", "free", "access mode: free or private")
	jsonOut := fs.Bool("json", false, "JSON output")
	dataDir := fs.String("datadir", config.DefaultDataDir(), "data directory")
	password := fs.String("password", "", "wallet password (for testing)")

	if err := fs.Parse(args); err != nil {
		return exitUsageError
	}

	if fs.NArg() < 2 {
		fmt.Fprintf(os.Stderr, "Usage: bitfs put <local-file> <remote-path> [--vault N] [--access free|private] [--json]\n")
		return exitUsageError
	}

	localFile := fs.Arg(0)
	remotePath := fs.Arg(1)

	if *access != "free" && *access != "private" {
		if *jsonOut {
			return writeJSONErr("put", exitUsageError, fmt.Errorf("--access must be 'free' or 'private'"))
		}
		fmt.Fprintf(os.Stderr, "Error: --access must be 'free' or 'private'\n")
		return exitUsageError
	}

	// Verify local file exists.
	if _, err := os.Stat(localFile); os.IsNotExist(err) {
		if *jsonOut {
			return writeJSONErr("put", exitNotFound, fmt.Errorf("local file %q not found", localFile))
		}
		fmt.Fprintf(os.Stderr, "Error: local file %q not found\n", localFile)
		return exitNotFound
	}

	pass, err := resolvePassword(*password)
	if err != nil {
		if *jsonOut {
			return writeJSONErr("put", exitWalletError, err)
		}
		printError(err)
		return exitWalletError
	}

	eng, err := vault.New(*dataDir, pass)
	if err != nil {
		if *jsonOut {
			return writeJSONErr("put", exitWalletError, err)
		}
		printError(err)
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
			return writeJSONErr("put", exitNotFound, err)
		}
		printError(err)
		return exitNotFound
	}

	result, err := eng.PutFile(&vault.PutOpts{
		VaultIndex: vaultIdx,
		LocalFile:  localFile,
		RemotePath: remotePath,
		Access:     *access,
	})
	if err != nil {
		if *jsonOut {
			return writeJSONErr("put", exitError, err)
		}
		printError(err)
		return exitError
	}


	if *jsonOut {
		return writeJSONResult(&cmdResult{
			OK:      true,
			Command: "put",
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
