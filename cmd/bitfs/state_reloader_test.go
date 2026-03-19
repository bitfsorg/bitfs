package main

import (
	"path/filepath"
	"testing"
	"time"
)

func TestVaultWalletAdapter_ReloadState(t *testing.T) {
	dataDir := initTestWallet(t)
	v := openTestVault(t, dataDir)
	defer v.Close()

	adapter := newVaultWalletAdapter(v)
	if _, err := adapter.GetVaultPubKey("alice"); err == nil {
		t.Fatal("expected alias alice to be absent before reload")
	}

	statePath := filepath.Join(dataDir, "state.json")
	state, err := loadWalletState(statePath)
	if err != nil {
		t.Fatalf("loadWalletState: %v", err)
	}
	if err := v.Wallet.BindPaymail(state, "alice", "default"); err != nil {
		t.Fatalf("BindPaymail: %v", err)
	}
	if err := saveWalletState(statePath, state); err != nil {
		t.Fatalf("saveWalletState: %v", err)
	}

	if err := adapter.ReloadState(); err != nil {
		t.Fatalf("ReloadState: %v", err)
	}

	pub, err := adapter.GetVaultPubKey("alice")
	if err != nil {
		t.Fatalf("GetVaultPubKey after reload: %v", err)
	}
	if len(pub) != 66 {
		t.Fatalf("pubkey length = %d, want 66", len(pub))
	}
}

func TestWalletStateAutoReloader(t *testing.T) {
	dataDir := initTestWallet(t)
	v := openTestVault(t, dataDir)
	defer v.Close()

	adapter := newVaultWalletAdapter(v)
	statePath := filepath.Join(dataDir, "state.json")
	reloader := startWalletStateAutoReloader(statePath, adapter)
	defer reloader.Close()

	state, err := loadWalletState(statePath)
	if err != nil {
		t.Fatalf("loadWalletState: %v", err)
	}
	if err := v.Wallet.BindPaymail(state, "bob", "default"); err != nil {
		t.Fatalf("BindPaymail: %v", err)
	}
	if err := saveWalletState(statePath, state); err != nil {
		t.Fatalf("saveWalletState: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if pub, err := adapter.GetVaultPubKey("bob"); err == nil && len(pub) == 66 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("auto reloader did not pick up updated state.json in time")
}
