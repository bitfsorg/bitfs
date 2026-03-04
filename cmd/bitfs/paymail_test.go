package main

import "testing"

func TestRunPaymailBindListUnbind(t *testing.T) {
	dataDir := initTestWallet(t)

	code := runPaymailBind([]string{"--datadir", dataDir, "--password", "testpass", "alice", "default"})
	if code != exitSuccess {
		t.Fatalf("runPaymailBind = %d, want %d", code, exitSuccess)
	}

	code = runPaymailList([]string{"--datadir", dataDir, "--password", "testpass"})
	if code != exitSuccess {
		t.Fatalf("runPaymailList = %d, want %d", code, exitSuccess)
	}

	code = runPaymailUnbind([]string{"--datadir", dataDir, "--password", "testpass", "alice"})
	if code != exitSuccess {
		t.Fatalf("runPaymailUnbind = %d, want %d", code, exitSuccess)
	}
}

func TestRunPaymailBind_DuplicateAlias(t *testing.T) {
	dataDir := initTestWallet(t)

	code := runPaymailBind([]string{"--datadir", dataDir, "--password", "testpass", "alice", "default"})
	if code != exitSuccess {
		t.Fatalf("first runPaymailBind = %d, want %d", code, exitSuccess)
	}

	// Create another vault so alias conflict is tested independently.
	if code := runVaultCreate([]string{"--datadir", dataDir, "--password", "testpass", "team"}); code != exitSuccess {
		t.Fatalf("runVaultCreate = %d, want %d", code, exitSuccess)
	}

	code = runPaymailBind([]string{"--datadir", dataDir, "--password", "testpass", "alice", "team"})
	if code != exitConflict {
		t.Fatalf("second runPaymailBind = %d, want %d", code, exitConflict)
	}
}

func TestRunPaymailBind_InvalidAlias(t *testing.T) {
	dataDir := initTestWallet(t)

	code := runPaymailBind([]string{"--datadir", dataDir, "--password", "testpass", "Alice", "default"})
	if code != exitUsageError {
		t.Fatalf("runPaymailBind invalid alias = %d, want %d", code, exitUsageError)
	}
}

func TestRunPaymailUnbind_NotFound(t *testing.T) {
	dataDir := initTestWallet(t)

	code := runPaymailUnbind([]string{"--datadir", dataDir, "--password", "testpass", "unknown"})
	if code != exitNotFound {
		t.Fatalf("runPaymailUnbind unknown = %d, want %d", code, exitNotFound)
	}
}
