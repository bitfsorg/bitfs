// Copyright (c) 2024 The BitFS developers
// Use of this source code is governed by the Open BSV License v5
// that can be found in the LICENSE file.

package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/bitfsorg/libbitfs-go/config"
	"github.com/bitfsorg/libbitfs-go/wallet"
)

// runStatus handles the "bitfs status" command — a quick overview of
// the current wallet, vaults, and daemon state.
func runStatus(args []string) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	dataDir := fs.String("datadir", config.DefaultDataDir(), "data directory")
	password := fs.String("password", "", "wallet password (for testing)")
	network := addNetworkFlag(fs)

	for _, a := range args {
		if a == "--help" || a == "-h" {
			fmt.Fprintf(os.Stderr, `bitfs status — Show wallet, vault, and daemon overview

Usage:
  bitfs status [options]

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

	// Check if wallet exists.
	walletPath := filepath.Join(*dataDir, "wallet.enc")
	if _, err := os.Stat(walletPath); os.IsNotExist(err) {
		fmt.Printf("Wallet:   not initialized\n")
		fmt.Printf("Data dir: %s\n", *dataDir)
		fmt.Printf("\nRun 'bitfs wallet init' to get started.\n")
		return exitSuccess
	}

	// Load wallet.
	pass, err := resolvePassword(*password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", userMessage(err))
		return exitWalletError
	}

	w, state, err := loadWalletFromDataDir(*dataDir, pass)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", userMessage(err))
		return exitWalletError
	}

	// Network.
	fmt.Printf("Network:  %s\n", w.Network().Name)
	fmt.Printf("Data dir: %s\n", *dataDir)

	// Fee address.
	feeKey, err := w.DeriveFeeKey(wallet.ExternalChain, 0)
	if err == nil {
		fmt.Printf("Address:  %s\n", hex.EncodeToString(feeKey.PublicKey.Compressed()))
	}

	// Vaults.
	vaults := w.ListVaults(state)
	fmt.Printf("Vaults:   %d", len(vaults))
	if len(vaults) > 0 {
		fmt.Printf(" (")
		for i, v := range vaults {
			if i > 0 {
				fmt.Printf(", ")
			}
			if i >= 3 {
				fmt.Printf("...")
				break
			}
			fmt.Printf("%s", v.Name)
		}
		fmt.Printf(")")
	}
	fmt.Println()

	// Daemon status.
	pidPath := filepath.Join(*dataDir, "daemon.pid")
	if pidData, pidErr := os.ReadFile(pidPath); pidErr == nil {
		pid, _ := strconv.Atoi(string(pidData))
		if pid > 0 {
			proc, _ := os.FindProcess(pid)
			if proc != nil && proc.Signal(syscall.Signal(0)) == nil {
				fmt.Printf("Daemon:   running (PID %d)\n", pid)
			} else {
				fmt.Printf("Daemon:   stopped (stale PID file)\n")
			}
		}
	} else {
		fmt.Printf("Daemon:   stopped\n")
	}

	return exitSuccess
}

func formatSatsCompact(sats uint64) string {
	if sats == 0 {
		return "0"
	}
	s := fmt.Sprintf("%d", sats)
	// Add comma separators for readability.
	n := len(s)
	if n <= 3 {
		return s
	}
	result := make([]byte, 0, n+(n-1)/3)
	for i, c := range s {
		if i > 0 && (n-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(c))
	}
	return string(result)
}
