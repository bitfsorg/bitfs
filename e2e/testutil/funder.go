//go:build e2e

package testutil

import (
	"context"
	"fmt"
)

// fundFromWIF imports a pre-funded WIF private key into the node's wallet
// and sends the specified amount to the target address.
// Returns the funding transaction ID.
func fundFromWIF(ctx context.Context, rpc *RPCClient, wif, addr string, amount float64) (string, error) {
	// Import the WIF private key without rescanning (we assume it's already known
	// or has been imported before; rescan=false keeps it fast).
	if err := rpc.Call(ctx, "importprivkey", []interface{}{wif, "", false}, nil); err != nil {
		return "", fmt.Errorf("importprivkey: %w", err)
	}

	// Send the requested amount to the target address.
	var txid string
	if err := rpc.Call(ctx, "sendtoaddress", []interface{}{addr, amount}, &txid); err != nil {
		return "", fmt.Errorf("sendtoaddress(%s, %f): %w", addr, amount, err)
	}

	return txid, nil
}
