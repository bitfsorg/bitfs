// Copyright (c) 2024 The BitFS developers
// Use of this source code is governed by the Open BSV License v5
// that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/bitfsorg/bitfs/internal/publish"
	"github.com/bitfsorg/libbitfs-go/config"
	"github.com/bitfsorg/libbitfs-go/vault"
)

// runUnpublish handles the "bitfs unpublish" command.
func runUnpublish(args []string) int {
	fs := flag.NewFlagSet("unpublish", flag.ContinueOnError)
	dataDir := fs.String("datadir", config.DefaultDataDir(), "data directory")
	password := fs.String("password", "", "wallet password (for testing)")
	network := addNetworkFlag(fs)

	for _, a := range args {
		if a == "--help" || a == "-h" {
			fmt.Fprintf(os.Stderr, `bitfs unpublish — Remove a domain binding

Usage:
  bitfs unpublish [options] <domain>

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
		fmt.Fprintf(os.Stderr, "Usage: bitfs unpublish <domain>\n")
		return exitUsageError
	}

	domain := fs.Arg(0)

	pass, err := resolvePassword(*password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", userMessage(err))
		return exitWalletError
	}

	v, err := vault.New(*dataDir, pass)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", userMessage(err))
		return exitWalletError
	}
	defer func() { _ = v.Close() }()

	result, err := publish.Unpublish(v, &publish.UnpublishOpts{
		Domain: domain,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", userMessage(err))
		return exitError
	}

	fmt.Println(result.Message)
	return exitSuccess
}
