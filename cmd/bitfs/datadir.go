// Copyright (c) 2024 The BitFS developers
// Use of this source code is governed by the Open BSV License v5
// that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bitfsorg/libbitfs-go/config"
)

var migrationHintOnce sync.Map

// applyNetworkDefaultDataDir rewrites *dataDir to a network-specific default
// when --datadir was not explicitly passed.
func applyNetworkDefaultDataDir(fs *flag.FlagSet, dataDir *string, network string) {
	dataDirSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "datadir" {
			dataDirSet = true
		}
	})
	if dataDirSet {
		return
	}

	// BITFS_DATADIR always wins when --datadir is omitted.
	if v := strings.TrimSpace(os.Getenv("BITFS_DATADIR")); v != "" {
		*dataDir = v
		return
	}

	*dataDir = config.DefaultDataDirForNetwork(strings.TrimSpace(network))
	maybeWarnLegacyDataDirMigration(network, *dataDir)
}

func maybeWarnLegacyDataDirMigration(network, selectedDataDir string) {
	net := strings.ToLower(strings.TrimSpace(network))
	if net != "testnet" && net != "regtest" {
		return
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return
	}

	legacyMainnetDir := filepath.Join(home, ".bitfs")
	selectedClean := filepath.Clean(selectedDataDir)
	if selectedClean == filepath.Clean(legacyMainnetDir) {
		return
	}

	if _, err := os.Stat(legacyMainnetDir); err != nil {
		return
	}
	if _, err := os.Stat(selectedClean); err == nil {
		return
	}

	key := net + ":" + selectedClean
	if _, loaded := migrationHintOnce.LoadOrStore(key, struct{}{}); loaded {
		return
	}

	fmt.Fprintf(os.Stderr, "Notice: found legacy data directory %s while using %s.\n", legacyMainnetDir, net)
	fmt.Fprintf(os.Stderr, "Default %s directory is now %s.\n", net, selectedClean)
	fmt.Fprintf(os.Stderr, "If needed, migrate manually: cp -R %s %s\n", legacyMainnetDir, selectedClean)
	fmt.Fprintf(os.Stderr, "Then verify permissions and rollback plan before switching production usage.\n")
}
