//go:build e2e

package testutil

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"
)

// liveNode implements TestNode for testnet and mainnet BSV networks.
// Unlike RegtestNode, it cannot mine blocks and must wait for real
// confirmations by polling the node.
type liveNode struct {
	rpc    *RPCClient
	config *Config
}

// Compile-time interface check.
var _ TestNode = (*liveNode)(nil)

// newLiveNode creates a liveNode with the given RPC client and config.
func newLiveNode(rpc *RPCClient, cfg *Config) *liveNode {
	return &liveNode{rpc: rpc, config: cfg}
}

// Network returns the configured network name ("testnet" or "mainnet").
func (n *liveNode) Network() string { return n.config.Network }

// RPC returns the underlying RPCClient.
func (n *liveNode) RPC() *RPCClient { return n.rpc }

// IsAvailable returns true if the node is reachable and responding.
func (n *liveNode) IsAvailable(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var hash string
	err := n.rpc.Call(ctx, "getbestblockhash", nil, &hash)
	return err == nil && hash != ""
}

// Fund sends the specified amount to addr using the configured funding strategy.
// On testnet, it tries faucet first (if configured), then falls back to WIF wallet.
// On mainnet, it only uses WIF wallet (faucet is never used).
// After funding, it waits for 1 confirmation.
func (n *liveNode) Fund(ctx context.Context, addr string, amount float64) (*UTXO, error) {
	var txid string
	funded := false

	// Try faucet first (testnet only, when configured).
	if n.config.FaucetURL != "" && !n.config.IsMainnet() {
		if err := faucetFund(ctx, n.config.FaucetURL, addr, amount); err == nil {
			funded = true
		}
		// If faucet fails, fall through to WIF.
	}

	// Try WIF wallet if faucet didn't work.
	if !funded && n.config.FundWIF != "" {
		var err error
		txid, err = fundFromWIF(ctx, n.rpc, n.config.FundWIF, addr, amount)
		if err != nil {
			return nil, fmt.Errorf("fund from WIF: %w", err)
		}
		funded = true
	}

	if !funded {
		return nil, fmt.Errorf("no funding source available on %s (set BITFS_E2E_FAUCET_URL or BITFS_E2E_FUND_WIF)", n.config.Network)
	}

	// Wait for the funding tx to confirm if we have a txid.
	if txid != "" {
		if err := n.WaitForConfirmation(ctx, txid, 1); err != nil {
			return nil, fmt.Errorf("wait for funding confirmation: %w", err)
		}
	} else {
		// Faucet doesn't return txid; poll ListUnspent directly.
		utxo, err := n.waitForUTXO(ctx, addr)
		if err != nil {
			return nil, fmt.Errorf("wait for faucet UTXO: %w", err)
		}
		return utxo, nil
	}

	// Retrieve the confirmed UTXO.
	utxos, err := n.ListUnspent(ctx, addr)
	if err != nil {
		return nil, fmt.Errorf("list unspent: %w", err)
	}
	if len(utxos) == 0 {
		return nil, fmt.Errorf("no UTXOs found for %s after funding", addr)
	}
	return &utxos[0], nil
}

// WaitForConfirmation polls getrawtransaction until the given txid has at
// least minConf confirmations. It uses config.ConfirmTimeout as the deadline.
func (n *liveNode) WaitForConfirmation(ctx context.Context, txid string, minConf int) error {
	deadline := time.After(n.config.ConfirmTimeout)
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		confs, err := n.getConfirmations(ctx, txid)
		if err == nil && confs >= minConf {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("timeout waiting for %d confirmations on %s (got %d after %v)",
				minConf, txid, confs, n.config.ConfirmTimeout)
		case <-ticker.C:
			// poll again
		}
	}
}

// MineBlocks is not supported on live networks and always returns an error.
func (n *liveNode) MineBlocks(_ context.Context, _ int, _ string) ([]string, error) {
	return nil, fmt.Errorf("mining not supported on %s", n.config.Network)
}

// --- RPC pass-through methods (identical to RegtestNode) ---

// NewAddress generates a new address from the node's wallet.
func (n *liveNode) NewAddress(ctx context.Context) (string, error) {
	var addr string
	if err := n.rpc.Call(ctx, "getnewaddress", nil, &addr); err != nil {
		return "", fmt.Errorf("getnewaddress: %w", err)
	}
	return addr, nil
}

// ListUnspent returns the unspent outputs for the given address.
func (n *liveNode) ListUnspent(ctx context.Context, addr string) ([]UTXO, error) {
	var utxos []UTXO
	params := []interface{}{1, 9999999, []string{addr}}
	if err := n.rpc.Call(ctx, "listunspent", params, &utxos); err != nil {
		return nil, fmt.Errorf("listunspent(%s): %w", addr, err)
	}
	return utxos, nil
}

// SendRawTransaction broadcasts a signed raw transaction hex to the network.
func (n *liveNode) SendRawTransaction(ctx context.Context, rawTxHex string) (string, error) {
	var txid string
	params := []interface{}{rawTxHex}
	if err := n.rpc.Call(ctx, "sendrawtransaction", params, &txid); err != nil {
		return "", fmt.Errorf("sendrawtransaction: %w", err)
	}
	return txid, nil
}

// SendToAddress sends the given BTC amount to addr from the node's wallet.
func (n *liveNode) SendToAddress(ctx context.Context, addr string, amount float64) (string, error) {
	var txid string
	params := []interface{}{addr, amount}
	if err := n.rpc.Call(ctx, "sendtoaddress", params, &txid); err != nil {
		return "", fmt.Errorf("sendtoaddress(%s, %f): %w", addr, amount, err)
	}
	return txid, nil
}

// GetRawTransaction returns the raw transaction bytes for the given txid.
func (n *liveNode) GetRawTransaction(ctx context.Context, txid string) ([]byte, error) {
	var rawHex string
	params := []interface{}{txid, false}
	if err := n.rpc.Call(ctx, "getrawtransaction", params, &rawHex); err != nil {
		return nil, fmt.Errorf("getrawtransaction(%s): %w", txid, err)
	}
	raw, err := hex.DecodeString(rawHex)
	if err != nil {
		return nil, fmt.Errorf("decode raw tx hex: %w", err)
	}
	return raw, nil
}

// GetBlockHeader returns the raw 80-byte block header for the given block hash.
func (n *liveNode) GetBlockHeader(ctx context.Context, blockHash string) ([]byte, error) {
	var headerHex string
	params := []interface{}{blockHash, false}
	if err := n.rpc.Call(ctx, "getblockheader", params, &headerHex); err != nil {
		return nil, fmt.Errorf("getblockheader(%s): %w", blockHash, err)
	}
	header, err := hex.DecodeString(headerHex)
	if err != nil {
		return nil, fmt.Errorf("decode block header hex: %w", err)
	}
	return header, nil
}

// GetBlockHeaderVerbose returns the verbose block header info as a map.
func (n *liveNode) GetBlockHeaderVerbose(ctx context.Context, blockHash string) (map[string]interface{}, error) {
	var result map[string]interface{}
	params := []interface{}{blockHash, true}
	if err := n.rpc.Call(ctx, "getblockheader", params, &result); err != nil {
		return nil, fmt.Errorf("getblockheader verbose(%s): %w", blockHash, err)
	}
	return result, nil
}

// GetTxOutProof returns the BIP37-style Merkle proof for the given transaction.
func (n *liveNode) GetTxOutProof(ctx context.Context, txid string) ([]byte, error) {
	var proofHex string
	params := []interface{}{[]string{txid}}
	if err := n.rpc.Call(ctx, "gettxoutproof", params, &proofHex); err != nil {
		return nil, fmt.Errorf("gettxoutproof(%s): %w", txid, err)
	}
	proof, err := hex.DecodeString(proofHex)
	if err != nil {
		return nil, fmt.Errorf("decode txout proof hex: %w", err)
	}
	return proof, nil
}

// GetBestBlockHash returns the hash of the best (tip) block.
func (n *liveNode) GetBestBlockHash(ctx context.Context) (string, error) {
	var hash string
	if err := n.rpc.Call(ctx, "getbestblockhash", nil, &hash); err != nil {
		return "", fmt.Errorf("getbestblockhash: %w", err)
	}
	return hash, nil
}

// GetBlockHash returns the block hash at the given height.
func (n *liveNode) GetBlockHash(ctx context.Context, height int) (string, error) {
	var hash string
	params := []interface{}{height}
	if err := n.rpc.Call(ctx, "getblockhash", params, &hash); err != nil {
		return "", fmt.Errorf("getblockhash(%d): %w", height, err)
	}
	return hash, nil
}

// GetBlockCount returns the current block height.
func (n *liveNode) GetBlockCount(ctx context.Context) (int64, error) {
	var count int64
	if err := n.rpc.Call(ctx, "getblockcount", nil, &count); err != nil {
		return 0, fmt.Errorf("getblockcount: %w", err)
	}
	return count, nil
}

// ImportAddress imports an address (or script) for watch-only tracking.
func (n *liveNode) ImportAddress(ctx context.Context, addr string) error {
	params := []interface{}{addr, "", false}
	if err := n.rpc.Call(ctx, "importaddress", params, nil); err != nil {
		return fmt.Errorf("importaddress(%s): %w", addr, err)
	}
	return nil
}

// --- Internal helpers ---

// getConfirmations queries getrawtransaction in verbose mode and returns
// the number of confirmations for the given txid.
func (n *liveNode) getConfirmations(ctx context.Context, txid string) (int, error) {
	var result map[string]interface{}
	params := []interface{}{txid, true}
	if err := n.rpc.Call(ctx, "getrawtransaction", params, &result); err != nil {
		return 0, err
	}
	confs, ok := result["confirmations"]
	if !ok {
		return 0, nil // unconfirmed
	}
	// JSON numbers decode as float64.
	switch v := confs.(type) {
	case float64:
		return int(v), nil
	default:
		return 0, fmt.Errorf("unexpected confirmations type: %T", confs)
	}
}

// waitForUTXO polls ListUnspent until at least one UTXO appears for the
// given address, using config.ConfirmTimeout as the deadline.
func (n *liveNode) waitForUTXO(ctx context.Context, addr string) (*UTXO, error) {
	// Import the address first so the node can track it.
	_ = n.ImportAddress(ctx, addr)

	deadline := time.After(n.config.ConfirmTimeout)
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		utxos, err := n.ListUnspent(ctx, addr)
		if err == nil && len(utxos) > 0 {
			return &utxos[0], nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline:
			return nil, fmt.Errorf("timeout waiting for UTXO at %s after %v", addr, n.config.ConfirmTimeout)
		case <-ticker.C:
			// poll again
		}
	}
}
