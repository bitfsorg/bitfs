// Copyright (c) 2024 The BitFS developers
// Use of this source code is governed by the Open BSV License v5
// that can be found in the LICENSE file.

package main

import (
	"flag"
	"strings"
	"testing"

	"github.com/bitfsorg/libbitfs-go/config"
)

func TestApplyNetworkDefaultDataDir_WhenDatadirNotSet(t *testing.T) {
	t.Setenv("BITFS_DATADIR", "")
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	dataDir := fs.String("datadir", config.DefaultDataDir(), "data directory")
	if err := fs.Parse(nil); err != nil {
		t.Fatalf("parse flags: %v", err)
	}

	applyNetworkDefaultDataDir(fs, dataDir, "testnet")
	if !strings.HasSuffix(*dataDir, ".bitfs-testnet") {
		t.Fatalf("datadir = %q, want suffix %q", *dataDir, ".bitfs-testnet")
	}

	applyNetworkDefaultDataDir(fs, dataDir, "regtest")
	if !strings.HasSuffix(*dataDir, ".regtest") {
		t.Fatalf("datadir = %q, want suffix %q", *dataDir, ".regtest")
	}
}

func TestApplyNetworkDefaultDataDir_WhenDatadirSet(t *testing.T) {
	t.Setenv("BITFS_DATADIR", "")
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	dataDir := fs.String("datadir", config.DefaultDataDir(), "data directory")
	if err := fs.Parse([]string{"--datadir", "/tmp/custom-bitfs"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}

	applyNetworkDefaultDataDir(fs, dataDir, "testnet")
	if *dataDir != "/tmp/custom-bitfs" {
		t.Fatalf("datadir = %q, want %q", *dataDir, "/tmp/custom-bitfs")
	}
}

func TestApplyNetworkDefaultDataDir_EnvOverride(t *testing.T) {
	t.Setenv("BITFS_DATADIR", "/tmp/env-bitfs")
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	dataDir := fs.String("datadir", config.DefaultDataDir(), "data directory")
	if err := fs.Parse(nil); err != nil {
		t.Fatalf("parse flags: %v", err)
	}

	applyNetworkDefaultDataDir(fs, dataDir, "testnet")
	if *dataDir != "/tmp/env-bitfs" {
		t.Fatalf("datadir = %q, want %q", *dataDir, "/tmp/env-bitfs")
	}
}
