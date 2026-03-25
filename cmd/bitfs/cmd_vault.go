// Copyright (c) 2024 The BitFS developers
// Use of this source code is governed by the Open BSV License v5
// that can be found in the LICENSE file.

package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/bitfsorg/libbitfs-go/config"
)

// runVault dispatches vault subcommands.
func runVault(args []string) int {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: bitfs vault <create|list|rename|delete|export> [options]\n")
		return exitUsageError
	}

	sub := args[0]
	subArgs := args[1:]

	switch sub {
	case "create":
		return runVaultCreate(subArgs)
	case "list":
		return runVaultList(subArgs)
	case "rename":
		return runVaultRename(subArgs)
	case "delete":
		return runVaultDelete(subArgs)
	case "export":
		return runVaultExport(subArgs)
	case "--help", "-h":
		fmt.Fprintf(os.Stderr, "Usage: bitfs vault <create|list|rename|delete|export> [options]\n\n")
		fmt.Fprintf(os.Stderr, "Subcommands:\n")
		fmt.Fprintf(os.Stderr, "  create <name>       Create a new vault\n")
		fmt.Fprintf(os.Stderr, "  list                List all vaults\n")
		fmt.Fprintf(os.Stderr, "  rename <old> <new>  Rename a vault\n")
		fmt.Fprintf(os.Stderr, "  delete <name>       Delete a vault\n")
		fmt.Fprintf(os.Stderr, "  export <name>       Export vault root private key\n")
		return exitSuccess
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown vault subcommand %q\n", sub)
		return exitUsageError
	}
}

// runVaultCreate creates a new vault.
func runVaultCreate(args []string) int {
	fs := flag.NewFlagSet("vault create", flag.ContinueOnError)
	dataDir := fs.String("datadir", config.DefaultDataDir(), "data directory")
	password := fs.String("password", "", "wallet password (for testing)")
	network := addNetworkFlag(fs)

	if err := fs.Parse(args); err != nil {
		return exitUsageError
	}
	if !resolveNetworkDataDir(fs, network, dataDir) {
		return exitUsageError
	}

	if fs.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "Usage: bitfs vault create <name>\n")
		return exitUsageError
	}

	name := strings.TrimSpace(fs.Arg(0))
	if name == "" {
		fmt.Fprintf(os.Stderr, "Error: vault name cannot be empty\n")
		return exitUsageError
	}
	if len(name) > 64 {
		fmt.Fprintf(os.Stderr, "Error: vault name too long (max 64 characters)\n")
		return exitUsageError
	}

	w, state, err := loadWalletFromDataDir(*dataDir, *password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", userMessage(err))
		return exitWalletError
	}

	vault, err := w.CreateVault(state, name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", userMessage(err))
		return exitConflict
	}

	statePath := *dataDir + "/state.json"
	if err := saveWalletState(statePath, state); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to save state: %v\n", err)
		return exitError
	}

	rootKey, err := w.DeriveVaultRootKey(vault.AccountIndex)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to derive vault root key: %v\n", err)
		return exitWalletError
	}

	fmt.Printf("Vault %q created.\n", name)
	fmt.Printf("  Account index: %d\n", vault.AccountIndex)
	fmt.Printf("  Root key path: %s\n", rootKey.Path)
	fmt.Printf("  Root pubkey:   %s\n", hex.EncodeToString(rootKey.PublicKey.Compressed()))

	return exitSuccess
}

// runVaultList lists all active vaults.
func runVaultList(args []string) int {
	fs := flag.NewFlagSet("vault list", flag.ContinueOnError)
	dataDir := fs.String("datadir", config.DefaultDataDir(), "data directory")
	password := fs.String("password", "", "wallet password (for testing)")
	network := addNetworkFlag(fs)

	if err := fs.Parse(args); err != nil {
		return exitUsageError
	}
	if !resolveNetworkDataDir(fs, network, dataDir) {
		return exitUsageError
	}

	w, state, err := loadWalletFromDataDir(*dataDir, *password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", userMessage(err))
		return exitWalletError
	}

	vaults := w.ListVaults(state)

	if len(vaults) == 0 {
		fmt.Printf("No vaults found. Run 'bitfs vault create <name>' to create one.\n")
		return exitSuccess
	}

	fmt.Printf("Vaults (%d):\n", len(vaults))
	for _, v := range vaults {
		rootKey, err := w.DeriveVaultRootKey(v.AccountIndex)
		if err != nil {
			fmt.Printf("  - %s (account %d, root key error)\n", v.Name, v.AccountIndex)
			continue
		}
		pubHex := hex.EncodeToString(rootKey.PublicKey.Compressed())
		fmt.Printf("  - %s (account %d, root %s...)\n", v.Name, v.AccountIndex, pubHex[:16])
	}

	return exitSuccess
}

// runVaultRename renames an existing vault.
func runVaultRename(args []string) int {
	fs := flag.NewFlagSet("vault rename", flag.ContinueOnError)
	dataDir := fs.String("datadir", config.DefaultDataDir(), "data directory")
	password := fs.String("password", "", "wallet password (for testing)")
	network := addNetworkFlag(fs)

	if err := fs.Parse(args); err != nil {
		return exitUsageError
	}
	if !resolveNetworkDataDir(fs, network, dataDir) {
		return exitUsageError
	}

	if fs.NArg() < 2 {
		fmt.Fprintf(os.Stderr, "Usage: bitfs vault rename <old-name> <new-name>\n")
		return exitUsageError
	}

	oldName := fs.Arg(0)
	newName := strings.TrimSpace(fs.Arg(1))
	if newName == "" {
		fmt.Fprintf(os.Stderr, "Error: vault name cannot be empty\n")
		return exitUsageError
	}
	if len(newName) > 64 {
		fmt.Fprintf(os.Stderr, "Error: vault name too long (max 64 characters)\n")
		return exitUsageError
	}

	w, state, err := loadWalletFromDataDir(*dataDir, *password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", userMessage(err))
		return exitWalletError
	}

	if err := w.RenameVault(state, oldName, newName); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", userMessage(err))
		return exitNotFound
	}

	statePath := *dataDir + "/state.json"
	if err := saveWalletState(statePath, state); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to save state: %v\n", err)
		return exitError
	}

	fmt.Printf("Vault renamed: %q -> %q\n", oldName, newName)
	return exitSuccess
}

// runVaultDelete soft-deletes a vault.
func runVaultDelete(args []string) int {
	fs := flag.NewFlagSet("vault delete", flag.ContinueOnError)
	dataDir := fs.String("datadir", config.DefaultDataDir(), "data directory")
	password := fs.String("password", "", "wallet password (for testing)")
	force := fs.Bool("force", false, "skip confirmation prompt")
	network := addNetworkFlag(fs)

	if err := fs.Parse(args); err != nil {
		return exitUsageError
	}
	if !resolveNetworkDataDir(fs, network, dataDir) {
		return exitUsageError
	}

	if fs.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "Usage: bitfs vault delete [--force] <name>\n")
		return exitUsageError
	}

	name := fs.Arg(0)

	if !*force {
		if !promptYesNo(fmt.Sprintf("Delete vault %q? This cannot be undone", name)) {
			fmt.Println("Aborted.")
			return exitSuccess
		}
	}

	w, state, err := loadWalletFromDataDir(*dataDir, *password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", userMessage(err))
		return exitWalletError
	}

	if err := w.DeleteVault(state, name); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", userMessage(err))
		return exitNotFound
	}

	statePath := *dataDir + "/state.json"
	if err := saveWalletState(statePath, state); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to save state: %v\n", err)
		return exitError
	}

	fmt.Printf("Vault %q deleted.\n", name)
	return exitSuccess
}
