// Copyright (c) 2024 The BitFS developers
// Use of this source code is governed by the Open BSV License v5
// that can be found in the LICENSE file.

package main

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/bitfsorg/libbitfs-go/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddNetworkFlag(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	network := addNetworkFlag(fs)
	require.NotNil(t, network)
	assert.Equal(t, "", *network, "default value should be empty string")

	err := fs.Parse([]string{"--network", "testnet"})
	require.NoError(t, err)
	assert.Equal(t, "testnet", *network)
}

func TestResolveNetworkDataDir_Testnet(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	dataDir := fs.String("datadir", config.DefaultDataDir(), "data directory")
	network := addNetworkFlag(fs)

	err := fs.Parse([]string{"--network", "testnet"})
	require.NoError(t, err)

	resolveNetworkDataDir(fs, network, dataDir)

	home, err := os.UserHomeDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".bitfs-testnet"), *dataDir)
}

func TestResolveNetworkDataDir_Regtest(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	dataDir := fs.String("datadir", config.DefaultDataDir(), "data directory")
	network := addNetworkFlag(fs)

	err := fs.Parse([]string{"--network", "regtest"})
	require.NoError(t, err)

	resolveNetworkDataDir(fs, network, dataDir)

	home, err := os.UserHomeDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".regtest"), *dataDir)
}

func TestResolveNetworkDataDir_ExplicitDataDirTakesPrecedence(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	dataDir := fs.String("datadir", config.DefaultDataDir(), "data directory")
	network := addNetworkFlag(fs)

	customDir := t.TempDir()
	err := fs.Parse([]string{"--network", "testnet", "--datadir", customDir})
	require.NoError(t, err)

	resolveNetworkDataDir(fs, network, dataDir)

	// Explicit --datadir should take precedence over --network.
	assert.Equal(t, customDir, *dataDir)
}

func TestResolveNetworkDataDir_EmptyNetworkIsNoop(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	dataDir := fs.String("datadir", config.DefaultDataDir(), "data directory")
	network := addNetworkFlag(fs)

	err := fs.Parse([]string{})
	require.NoError(t, err)

	original := *dataDir
	resolveNetworkDataDir(fs, network, dataDir)

	// No --network means no change.
	assert.Equal(t, original, *dataDir)
}

func TestResolveNetworkDataDir_Mainnet(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	dataDir := fs.String("datadir", config.DefaultDataDir(), "data directory")
	network := addNetworkFlag(fs)

	err := fs.Parse([]string{"--network", "mainnet"})
	require.NoError(t, err)

	resolveNetworkDataDir(fs, network, dataDir)

	home, err := os.UserHomeDir()
	require.NoError(t, err)
	// Mainnet uses the default ~/.bitfs directory.
	assert.Equal(t, filepath.Join(home, ".bitfs"), *dataDir)
}
