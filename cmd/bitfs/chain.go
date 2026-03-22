// Copyright (c) 2024 The BitFS developers
// Use of this source code is governed by the Open BSV License v5
// that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"

	"github.com/bitfsorg/libbitfs-go/network"
	"github.com/bitfsorg/libbitfs-go/vault"
)

// configureChain resolves RPC configuration and attaches a BlockchainService
// to the vault. If RPC is explicitly configured, it is used for any network.
// Otherwise, for mainnet/testnet, the composite WoC+ARC backend is selected
// automatically. If neither is available, the vault stays in offline mode.
func configureChain(v *vault.Vault, rpcURL, rpcUser, rpcPass, netName string, arcURLOverride ...string) {
	flags := &network.RPCConfig{
		URL:      rpcURL,
		User:     rpcUser,
		Password: rpcPass,
	}

	env := envToMap()
	cfg, err := network.ResolveConfig(flags, env, netName)
	if err == nil {
		// RPC explicitly configured — use it (any network).
		v.Chain = network.NewRPCClient(*cfg)
		return
	}

	// No RPC: try WoC+ARC for mainnet/testnet.
	wocBase, arcBase, wocErr := network.DefaultWoCARCEndpoints(netName)
	if wocErr != nil {
		fmt.Fprintf(os.Stderr, "Note: no RPC configured, running in offline mode (%v)\n", err)
		return
	}

	wocKey := os.Getenv("BITFS_WOC_API_KEY")
	arcKey := os.Getenv("BITFS_ARC_API_KEY")
	if len(arcURLOverride) > 0 && arcURLOverride[0] != "" {
		arcBase = arcURLOverride[0]
	} else if arcURL := os.Getenv("BITFS_ARC_URL"); arcURL != "" {
		arcBase = arcURL
	}

	v.Chain = network.NewWoCARCClient(network.WoCARCConfig{
		WoC: network.WoCConfig{BaseURL: wocBase, APIKey: wocKey},
		ARC: network.ARCConfig{BaseURL: arcBase, APIKey: arcKey},
	})
}

// envToMap reads BITFS_RPC_* environment variables into a map.
func envToMap() map[string]string {
	m := make(map[string]string)
	for _, key := range []string{"BITFS_RPC_URL", "BITFS_RPC_USER", "BITFS_RPC_PASS"} {
		if v := os.Getenv(key); v != "" {
			m[key] = v
		}
	}
	return m
}
