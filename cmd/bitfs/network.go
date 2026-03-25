// Copyright (c) 2024 The BitFS developers
// Use of this source code is governed by the Open BSV License v5
// that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"os"
)

// validNetworks lists the accepted --network values.
var validNetworks = map[string]bool{
	"mainnet": true,
	"testnet": true,
	"regtest": true,
}

// addNetworkFlag adds --network to a FlagSet and returns the pointer.
// After fs.Parse(), call resolveNetworkDataDir(fs, network, dataDir).
func addNetworkFlag(fs *flag.FlagSet) *string {
	return fs.String("network", "", "BSV network: mainnet, testnet, or regtest (auto-detected from datadir)")
}

// resolveNetworkDataDir applies --network to --datadir when datadir was not
// explicitly set. Returns false if the network name is invalid.
// Call after fs.Parse().
func resolveNetworkDataDir(fs *flag.FlagSet, network *string, dataDir *string) bool {
	if *network == "" {
		return true
	}
	if !validNetworks[*network] {
		fmt.Fprintf(os.Stderr, "Error: unknown network %q (must be mainnet, testnet, or regtest)\n", *network)
		return false
	}
	applyNetworkDefaultDataDir(fs, dataDir, *network)
	return true
}
