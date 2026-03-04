//go:build e2e

package testutil

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	libnetwork "github.com/bitfsorg/libbitfs-go/network"
)

// liveNode implements TestNode for testnet and mainnet BSV networks.
// Unlike RegtestNode, it cannot mine blocks.
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

// Fund sends the specified amount to addr from the configured funding WIF.
// On live networks BITFS_E2E_FUND_WIF is required.
func (n *liveNode) Fund(ctx context.Context, addr string, amount float64) (*UTXO, error) {
	if n.config.FundWIF == "" {
		return nil, fmt.Errorf("BITFS_E2E_FUND_WIF is required on %s", n.config.Network)
	}

	txid, rawHex, err := fundFromWIF(ctx, n.rpc, n.config.FundWIF, addr, amount)
	if err != nil {
		return nil, fmt.Errorf("fund from WIF: %w", err)
	}

	if err := n.WaitForConfirmation(ctx, txid, 1); err != nil {
		return nil, fmt.Errorf("wait for funding propagation: %w", err)
	}

	utxo, parseErr := fundingUTXOFromRawTx(rawHex, addr, txid)
	if parseErr == nil {
		return utxo, nil
	}

	// Fallback for providers where raw parsing fails unexpectedly.
	utxos, err := n.ListUnspent(ctx, addr)
	if err != nil {
		return nil, fmt.Errorf("list unspent: %w", err)
	}
	for i := range utxos {
		if strings.EqualFold(utxos[i].TxID, txid) {
			return &utxos[i], nil
		}
	}
	if len(utxos) > 0 {
		return &utxos[0], nil
	}
	return nil, fmt.Errorf("no UTXOs found for %s after funding (parse raw tx failed: %v)", addr, parseErr)
}

// WaitForConfirmation on live RPC nodes waits for transaction propagation,
// not mined confirmations. It returns once getrawtransaction can locate txid.
func (n *liveNode) WaitForConfirmation(ctx context.Context, txid string, minConf int) error {
	_ = minConf // live providers don't require confirmed blocks for most e2e flows

	deadline := time.After(n.config.ConfirmTimeout)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		_, err := n.GetTxStatus(ctx, txid)
		if err == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("timeout waiting for tx propagation on %s after %v", txid, n.config.ConfirmTimeout)
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
	// Include unconfirmed UTXOs on live networks to avoid mined-confirmation waits.
	params := []interface{}{0, 9999999, []string{addr}}
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
	if n.config.FundWIF != "" {
		var err error
		txid, _, err = fundFromWIF(ctx, n.rpc, n.config.FundWIF, addr, amount)
		if err != nil {
			return "", fmt.Errorf("fund from WIF: %w", err)
		}
		return txid, nil
	}
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

// GetTxStatus returns tx confirmation metadata from verbose getrawtransaction.
func (n *liveNode) GetTxStatus(ctx context.Context, txid string) (*TxStatus, error) {
	var result struct {
		Confirmations int64  `json:"confirmations"`
		BlockHash     string `json:"blockhash"`
		BlockHeight   uint64 `json:"blockheight"`
	}
	params := []interface{}{txid, true}
	if err := n.rpc.Call(ctx, "getrawtransaction", params, &result); err != nil {
		return nil, fmt.Errorf("getrawtransaction verbose(%s): %w", txid, err)
	}
	return &TxStatus{
		Confirmations: result.Confirmations,
		BlockHash:     result.BlockHash,
		BlockHeight:   result.BlockHeight,
	}, nil
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

// GetBlockTxIDs returns all txids in the block (display hex).
func (n *liveNode) GetBlockTxIDs(ctx context.Context, blockHash string) ([]string, error) {
	var result struct {
		Tx []string `json:"tx"`
	}
	params := []interface{}{blockHash, 1}
	if err := n.rpc.Call(ctx, "getblock", params, &result); err != nil {
		return nil, fmt.Errorf("getblock(%s): %w", blockHash, err)
	}
	return result.Tx, nil
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

// GetMerkleProof returns a normalized SPV merkle proof for txid.
func (n *liveNode) GetMerkleProof(ctx context.Context, txid string) (*MerkleProof, error) {
	proofBytes, err := n.GetTxOutProof(ctx, txid)
	if err != nil {
		return nil, err
	}

	txidBytes, err := hex.DecodeString(txid)
	if err != nil {
		return nil, fmt.Errorf("decode txid hex: %w", err)
	}
	reverseBytesInPlace(txidBytes)

	_, txIndex, branches, _, err := libnetwork.ParseBIP37MerkleBlock(proofBytes, txidBytes)
	if err != nil {
		return nil, fmt.Errorf("parse BIP37 merkle block: %w", err)
	}

	status, err := n.GetTxStatus(ctx, txid)
	if err != nil {
		return nil, err
	}

	return &MerkleProof{
		TxID:      txidBytes,
		Index:     txIndex,
		Nodes:     branches,
		BlockHash: status.BlockHash,
	}, nil
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
