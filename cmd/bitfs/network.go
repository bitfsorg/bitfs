// Copyright (c) 2024 The BitFS developers
// Use of this source code is governed by the Open BSV License v5
// that can be found in the LICENSE file.

package main

import "flag"

// addNetworkFlag adds --network to a FlagSet and returns the pointer.
// After fs.Parse(), call resolveNetworkDataDir(fs, network, dataDir).
func addNetworkFlag(fs *flag.FlagSet) *string {
	return fs.String("network", "", "BSV network: mainnet, testnet, or regtest (auto-detected from datadir)")
}

// resolveNetworkDataDir applies --network to --datadir when datadir was not
// explicitly set. Call after fs.Parse().
func resolveNetworkDataDir(fs *flag.FlagSet, network *string, dataDir *string) {
	if *network != "" {
		applyNetworkDefaultDataDir(fs, dataDir, *network)
	}
}
