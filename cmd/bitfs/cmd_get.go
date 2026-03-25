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

// runGet handles the "bitfs get" command.
func runGet(args []string) int {
	fs := flag.NewFlagSet("get", flag.ContinueOnError)
	vaultName := fs.String("vault", "", "vault name")
	dataDir := fs.String("datadir", config.DefaultDataDir(), "data directory")
	password := fs.String("password", "", "wallet password (for testing)")
	network := addNetworkFlag(fs)

	for _, a := range args {
		if a == "--help" || a == "-h" {
			fmt.Fprintf(os.Stderr, `bitfs get — Download a file from a BitFS vault

Usage:
  bitfs get [options] <remote-path> [local-path]

Options:
`)
			fs.SetOutput(os.Stderr)
			fs.PrintDefaults()
			return exitSuccess
		}
		if a == "--" {
			break
		}
	}

	if err := fs.Parse(args); err != nil {
		return exitUsageError
	}
	if !resolveNetworkDataDir(fs, network, dataDir) {
		return exitUsageError
	}

	if fs.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "Usage: bitfs get <remote-path> [local-path] [--vault N]\n")
		return exitUsageError
	}

	remotePath := fs.Arg(0)
	localPath := ""
	if fs.NArg() > 1 {
		localPath = fs.Arg(1)
	}

	pass, err := resolvePassword(*password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", userMessage(err))
		return exitWalletError
	}

	eng, err := vault.New(*dataDir, pass)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", userMessage(err))
		return exitWalletError
	}
	defer func() { _ = eng.Close() }()

	vaultIdx, err := eng.ResolveVaultIndex(*vaultName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", userMessage(err))
		return exitNotFound
	}

	localDir, _ := os.Getwd()
	result, err := eng.Get(&vault.GetOpts{
		VaultIndex: vaultIdx,
		RemotePath: remotePath,
		LocalDir:   localDir,
		LocalPath:  localPath,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", userMessage(err))
		return exitError
	}

	fmt.Println(result.Message)
	return exitSuccess
}
