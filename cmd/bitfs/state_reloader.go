// Copyright (c) 2024 The BitFS developers
// Use of this source code is governed by the Open BSV License v5
// that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
)

type walletStateAutoReloader struct {
	watcher *fsnotify.Watcher
	stop    chan struct{}
	done    chan struct{}
}

func startWalletStateAutoReloader(statePath string, adapter *vaultWalletAdapter) *walletStateAutoReloader {
	r := &walletStateAutoReloader{
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to initialize file watcher: %v\n", err)
		close(r.done)
		return r
	}
	r.watcher = watcher

	watchDir := filepath.Dir(statePath)
	if err := watcher.Add(watchDir); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to watch %s: %v\n", watchDir, err)
		_ = watcher.Close()
		close(r.done)
		return r
	}

	go func() {
		defer close(r.done)
		defer func() { _ = watcher.Close() }()

		target := filepath.Clean(statePath)
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if !isStateFileEvent(event, target) {
					continue
				}
				if err := adapter.ReloadState(); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to reload wallet state: %v\n", err)
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				fmt.Fprintf(os.Stderr, "Warning: file watcher error: %v\n", err)
			case <-r.stop:
				return
			}
		}
	}()

	return r
}

func (r *walletStateAutoReloader) Close() {
	close(r.stop)
	<-r.done
}

func isStateFileEvent(event fsnotify.Event, targetPath string) bool {
	if filepath.Clean(event.Name) != targetPath {
		return false
	}
	return event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename|fsnotify.Chmod) != 0
}
