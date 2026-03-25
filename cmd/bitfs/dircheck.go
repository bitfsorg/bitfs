// Copyright (c) 2024 The BitFS developers
// Use of this source code is governed by the Open BSV License v5
// that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"
	"runtime"
)

// warnDataDirPermissions prints a warning to stderr if the data directory
// has overly permissive mode bits (group or other readable/writable).
// Only checks on Unix-like systems.
func warnDataDirPermissions(dataDir string) {
	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Stat(dataDir)
	if err != nil {
		return
	}
	mode := info.Mode().Perm()
	if mode&0077 != 0 {
		fmt.Fprintf(os.Stderr, "WARNING: data directory %q has permissions %04o; should be 0700.\n", dataDir, mode)
		fmt.Fprintf(os.Stderr, "Fix with: chmod 700 %s\n", dataDir)
	}
}
