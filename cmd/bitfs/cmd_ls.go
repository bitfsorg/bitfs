// Copyright (c) 2024 The BitFS developers
// Use of this source code is governed by the Open BSV License v5
// that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/bitfsorg/libbitfs-go/config"
	"github.com/bitfsorg/libbitfs-go/vault"
)

// runLs handles the "bitfs ls" command.
func runLs(args []string) int {
	fs := flag.NewFlagSet("ls", flag.ContinueOnError)
	vaultName := fs.String("vault", "", "vault name")
	jsonOut := fs.Bool("json", false, "JSON output")
	long := fs.Bool("long", false, "detailed listing")
	fs.BoolVar(long, "l", false, "detailed listing (alias)")
	dataDir := fs.String("datadir", config.DefaultDataDir(), "data directory")
	password := fs.String("password", "", "wallet password (for testing)")
	network := addNetworkFlag(fs)

	for _, a := range args {
		if a == "--help" || a == "-h" {
			fmt.Fprintf(os.Stderr, `bitfs ls — List directory contents in a BitFS vault

Usage:
  bitfs ls [options] [path]

Options:
`)
			fs.SetOutput(os.Stderr)
			fs.PrintDefaults()
			return exitSuccess
		}
		if a == "--" {
			break
		}
	}

	if err := fs.Parse(args); err != nil {
		return exitUsageError
	}
	if !resolveNetworkDataDir(fs, network, dataDir) {
		return exitUsageError
	}

	remotePath := "/"
	if fs.NArg() > 0 {
		remotePath = fs.Arg(0)
	}

	pass, err := resolvePassword(*password)
	if err != nil {
		if *jsonOut {
			return writeJSONErr("ls", exitWalletError, err)
		}
		fmt.Fprintf(os.Stderr, "Error: %s\n", userMessage(err))
		return exitWalletError
	}

	eng, err := vault.New(*dataDir, pass)
	if err != nil {
		if *jsonOut {
			return writeJSONErr("ls", exitWalletError, err)
		}
		fmt.Fprintf(os.Stderr, "Error: %s\n", userMessage(err))
		return exitWalletError
	}
	defer func() { _ = eng.Close() }()

	if _, err := eng.ResolveVaultIndex(*vaultName); err != nil {
		if *jsonOut {
			return writeJSONErr("ls", exitNotFound, err)
		}
		fmt.Fprintf(os.Stderr, "Error: %s\n", userMessage(err))
		return exitNotFound
	}

	node := eng.State.FindNodeByPath(remotePath)
	if node == nil {
		if remotePath == "/" || remotePath == "" {
			// Empty vault root.
			if *jsonOut {
				fmt.Println("[]")
			}
			return exitSuccess
		}
		if *jsonOut {
			return writeJSONErr("ls", exitNotFound, fmt.Errorf("path %q not found", remotePath))
		}
		fmt.Fprintf(os.Stderr, "Error: path %q not found\n", remotePath)
		return exitNotFound
	}

	if node.Type != "dir" {
		// Single file — show its info.
		if *jsonOut {
			data, _ := json.Marshal(map[string]interface{}{
				"path": remotePath,
				"type": node.Type,
			})
			fmt.Println(string(data))
		} else if *long {
			fmt.Printf("%s\t%s\t%s\n", node.Type, node.Access, remotePath)
		} else {
			fmt.Println(remotePath)
		}
		return exitSuccess
	}

	if len(node.Children) == 0 {
		if *jsonOut {
			fmt.Println("[]")
		}
		return exitSuccess
	}

	if *jsonOut {
		type entry struct {
			Name string `json:"name"`
			Type string `json:"type"`
		}
		entries := make([]entry, len(node.Children))
		for i, c := range node.Children {
			entries[i] = entry{Name: c.Name, Type: c.Type}
		}
		data, _ := json.MarshalIndent(entries, "", "  ")
		fmt.Println(string(data))
		return exitSuccess
	}

	for _, c := range node.Children {
		name := c.Name
		if c.Type == "dir" {
			name += "/"
		}
		if *long {
			fmt.Printf("%s\t%s\n", c.Type, name)
		} else {
			fmt.Println(name)
		}
	}

	return exitSuccess
}
