// Copyright (c) 2024 The BitFS developers
// Use of this source code is governed by the Open BSV License v5
// that can be found in the LICENSE file.

package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

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

	w, state, err := loadWalletFromDataDir(*dataDir, *password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return exitWalletError
	}

	if err := w.BindPaymail(state, alias, vaultName); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		switch {
		case errors.Is(err, wallet.ErrVaultNotFound):
			return exitNotFound
		case errors.Is(err, wallet.ErrInvalidAlias):
			return exitUsageError
		default:
			return exitConflict
		}
	}

	statePath := *dataDir + "/state.json"
	if err := saveWalletState(statePath, state); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to save state: %v\n", err)
		return exitError
	}

	// Derive pubkey for display.
	vault, err := w.GetVault(state, vaultName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return exitError
	}

	rootKey, err := w.DeriveVaultRootKey(vault.AccountIndex)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to derive vault root key: %v\n", err)
		return exitWalletError
	}

	pubHex := hex.EncodeToString(rootKey.PublicKey.Compressed())
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

	w, state, err := loadWalletFromDataDir(*dataDir, *password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return exitWalletError
	}

	// Look up vault name for display before unbinding.
	vaultName, err := w.ResolvePaymailAlias(state, alias)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return exitNotFound
	}

	if err := w.UnbindPaymail(state, alias); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return exitNotFound
	}

	statePath := *dataDir + "/state.json"
	if err := saveWalletState(statePath, state); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to save state: %v\n", err)
		return exitError
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

	w, state, err := loadWalletFromDataDir(*dataDir, *password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return exitWalletError
	}

	bindings := w.ListPaymailBindings(state)

	if len(bindings) == 0 {
		if *jsonOut {
			fmt.Println("[]")
		} else {
			fmt.Println("No paymail bindings configured.")
		}
		return exitSuccess
	}

	// Build entries with pubkeys.
	entries := make([]paymailEntry, 0, len(bindings))
	for _, b := range bindings {
		pubHex := "(error)"
		vault, err := w.GetVault(state, b.Vault)
		if err == nil {
			rootKey, err := w.DeriveVaultRootKey(vault.AccountIndex)
			if err == nil {
				pubHex = hex.EncodeToString(rootKey.PublicKey.Compressed())
			}
		}
		entries = append(entries, paymailEntry{
			Alias:  b.Alias,
			Vault:  b.Vault,
			Pubkey: pubHex,
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
