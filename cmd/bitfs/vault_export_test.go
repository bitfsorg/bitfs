package main

import "testing"

func TestRunVaultExport_SeedPath(t *testing.T) {
	dataDir := initTestWallet(t)

	code := runVaultExport([]string{
		"--datadir", dataDir,
		"--password", "testpass",
		"--format", "seed-path",
		"--yes",
		"default",
	})
	if code != exitSuccess {
		t.Fatalf("runVaultExport seed-path = %d, want %d", code, exitSuccess)
	}
}

func TestRunVaultExport_InvalidFormat(t *testing.T) {
	dataDir := initTestWallet(t)

	code := runVaultExport([]string{
		"--datadir", dataDir,
		"--password", "testpass",
		"--format", "pem",
		"--yes",
		"default",
	})
	if code != exitError {
		t.Fatalf("runVaultExport invalid format = %d, want %d", code, exitError)
	}
}
