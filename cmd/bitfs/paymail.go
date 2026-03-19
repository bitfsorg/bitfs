// Copyright (c) 2024 The BitFS developers
// Use of this source code is governed by the Open BSV License v5
// that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/bitfsorg/bitfs/internal/engine"
	"github.com/bitfsorg/libbitfs-go/config"
	"github.com/bitfsorg/libbitfs-go/wallet"
)

// runPaymail dispatches paymail subcommands.
func runPaymail(args []string) int {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: bitfs paymail <bind|unbind|list> [options]\n")
		return exitUsageError
	}

	sub := args[0]
	subArgs := args[1:]

	switch sub {
	case "bind":
		return runPaymailBind(subArgs)
	case "unbind":
		return runPaymailUnbind(subArgs)
	case "list":
		return runPaymailList(subArgs)
	case "--help", "-h":
		fmt.Fprintf(os.Stderr, "Usage: bitfs paymail <bind|unbind|list> [options]\n\n")
		fmt.Fprintf(os.Stderr, "Subcommands:\n")
		fmt.Fprintf(os.Stderr, "  bind <alias> <vault>  Bind a vault to a paymail alias\n")
		fmt.Fprintf(os.Stderr, "  unbind <alias>        Remove a paymail alias binding\n")
		fmt.Fprintf(os.Stderr, "  list                  List all paymail bindings\n")
		return exitSuccess
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown paymail subcommand %q\n", sub)
		return exitUsageError
	}
}

// runPaymailBind binds a vault to a paymail alias.
func runPaymailBind(args []string) int {
	fs := flag.NewFlagSet("paymail bind", flag.ContinueOnError)
	dataDir := fs.String("datadir", config.DefaultDataDir(), "data directory")
	password := fs.String("password", "", "wallet password (for testing)")

	if err := fs.Parse(args); err != nil {
		return exitUsageError
	}

	if fs.NArg() < 2 {
		fmt.Fprintf(os.Stderr, "Usage: bitfs paymail bind <alias> <vault>\n")
		return exitUsageError
	}

	alias := fs.Arg(0)
	vaultName := fs.Arg(1)

	pass, err := resolvePassword(*password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return exitWalletError
	}

	eng := engine.New(*dataDir, pass)
	pubHex, err := eng.PaymailBind(alias, vaultName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		switch {
		case errors.Is(err, wallet.ErrVaultNotFound):
			return exitNotFound
		case errors.Is(err, wallet.ErrInvalidAlias):
			return exitUsageError
		case errors.Is(err, wallet.ErrAliasExists), errors.Is(err, wallet.ErrVaultAlreadyBound):
			return exitConflict
		default:
			return exitWalletError
		}
	}

	fmt.Printf("Bound paymail alias %q → vault %q (%s...)\n", alias, vaultName, pubHex[:16])

	return exitSuccess
}

// runPaymailUnbind removes a paymail alias binding.
func runPaymailUnbind(args []string) int {
	fs := flag.NewFlagSet("paymail unbind", flag.ContinueOnError)
	dataDir := fs.String("datadir", config.DefaultDataDir(), "data directory")
	password := fs.String("password", "", "wallet password (for testing)")

	if err := fs.Parse(args); err != nil {
		return exitUsageError
	}

	if fs.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "Usage: bitfs paymail unbind <alias>\n")
		return exitUsageError
	}

	alias := fs.Arg(0)

	pass, err := resolvePassword(*password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return exitWalletError
	}

	eng := engine.New(*dataDir, pass)
	vaultName, err := eng.PaymailUnbind(alias)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		if errors.Is(err, wallet.ErrAliasNotFound) {
			return exitNotFound
		}
		return exitWalletError
	}

	fmt.Printf("Unbound paymail alias %q (was → vault %q)\n", alias, vaultName)
	return exitSuccess
}

// paymailEntry is the JSON output format for paymail list.
type paymailEntry struct {
	Alias  string `json:"alias"`
	Vault  string `json:"vault"`
	Pubkey string `json:"pubkey"`
}

// runPaymailList lists all paymail bindings.
func runPaymailList(args []string) int {
	fs := flag.NewFlagSet("paymail list", flag.ContinueOnError)
	dataDir := fs.String("datadir", config.DefaultDataDir(), "data directory")
	password := fs.String("password", "", "wallet password (for testing)")
	jsonOut := fs.Bool("json", false, "output as JSON")

	if err := fs.Parse(args); err != nil {
		return exitUsageError
	}

	pass, err := resolvePassword(*password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return exitWalletError
	}

	eng := engine.New(*dataDir, pass)
	entriesRaw, err := eng.PaymailList()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return exitWalletError
	}
	if len(entriesRaw) == 0 {
		if *jsonOut {
			fmt.Println("[]")
		} else {
			fmt.Println("No paymail bindings configured.")
		}
		return exitSuccess
	}

	entries := make([]paymailEntry, 0, len(entriesRaw))
	for _, e := range entriesRaw {
		entries = append(entries, paymailEntry{
			Alias:  e.Alias,
			Vault:  e.Vault,
			Pubkey: e.Pubkey,
		})
	}

	if *jsonOut {
		data, err := json.MarshalIndent(entries, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return exitError
		}
		fmt.Println(string(data))
		return exitSuccess
	}

	// Table format.
	fmt.Printf("%-16s %-16s %s\n", "ALIAS", "VAULT", "PUBKEY")
	for _, e := range entries {
		fmt.Printf("%-16s %-16s %s\n", e.Alias, e.Vault, e.Pubkey)
	}

	return exitSuccess
}
