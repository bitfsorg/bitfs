// Copyright (c) 2024 The BitFS developers
// Use of this source code is governed by the Open BSV License v5
// that can be found in the LICENSE file.

package main

import (
	"flag"
	"strings"

	"github.com/bitfsorg/libbitfs-go/config"
)

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
	*dataDir = config.DefaultDataDirForNetwork(strings.TrimSpace(network))
}
