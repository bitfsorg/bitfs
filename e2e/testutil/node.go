//go:build e2e

package testutil

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"

	libnetwork "github.com/bitfsorg/libbitfs-go/network"
)

// TestNode is the unified interface for interacting with a BSV node across
// regtest, testnet, and mainnet networks. All e2e test helpers should accept
// TestNode rather than a concrete type.
type TestNode interface {
	// Network returns the network name: "regtest", "testnet", or "mainnet".
	Network() string
	// IsAvailable returns true if the node is reachable and responding.
	IsAvailable(ctx context.Context) bool
	// Fund sends the specified amount to addr and waits for confirmation.
	// On regtest it mines blocks; on live networks it uses the configured WIF wallet.
	Fund(ctx context.Context, addr string, amount float64) (*UTXO, error)
	// WaitForConfirmation waits until the given txid has at least minConf confirmations.
	// On regtest it mines blocks; on live networks it polls.
	WaitForConfirmation(ctx context.Context, txid string, minConf int) error

	// --- RPC pass-through methods ---

	SendRawTransaction(ctx context.Context, hex string) (string, error)
	GetRawTransaction(ctx context.Context, txid string) ([]byte, error)
	GetTxStatus(ctx context.Context, txid string) (*TxStatus, error)
	GetTxOutProof(ctx context.Context, txid string) ([]byte, error)
	GetMerkleProof(ctx context.Context, txid string) (*MerkleProof, error)
	GetBlockHeader(ctx context.Context, hash string) ([]byte, error)
	GetBlockHeaderVerbose(ctx context.Context, hash string) (map[string]interface{}, error)
	GetBlockTxIDs(ctx context.Context, hash string) ([]string, error)
	GetBestBlockHash(ctx context.Context) (string, error)
	GetBlockHash(ctx context.Context, height int) (string, error)
	GetBlockCount(ctx context.Context) (int64, error)
	ImportAddress(ctx context.Context, addr string) error
	ListUnspent(ctx context.Context, addr string) ([]UTXO, error)
	SendToAddress(ctx context.Context, addr string, amount float64) (string, error)
	NewAddress(ctx context.Context) (string, error)

	// MineBlocks mines count blocks sending rewards to addr.
	// Only supported on regtest; returns an error on live networks.
	MineBlocks(ctx context.Context, count int, addr string) ([]string, error)

	// RPC returns the underlying RPCClient for direct RPC calls.
	RPC() *RPCClient
}

// Compile-time interface check.
var _ TestNode = (*RegtestNode)(nil)

// RegtestNode provides high-level helpers around a BSV regtest JSON-RPC node.
type RegtestNode struct {
	rpc *RPCClient
}

// NewTestNode creates a TestNode appropriate for the configured network.
// It loads config from environment variables, selects regtest or live node,
// and skips the test if the node is not available.
func NewTestNode(t *testing.T) TestNode {
	t.Helper()
	return newTestNodeWithAvailabilityMode(t, true)
}

// NewRequiredTestNode creates a TestNode and fails the test if unavailable.
// Unlike NewTestNode, it never marks the test as skipped.
func NewRequiredTestNode(t *testing.T) TestNode {
	t.Helper()
	return newTestNodeWithAvailabilityMode(t, false)
}

func newTestNodeWithAvailabilityMode(t *testing.T, skipOnUnavailable bool) TestNode {
	t.Helper()
	cfg := LoadConfig()

	var node TestNode
	switch cfg.Network {
	case "testnet", "mainnet":
		if cfg.Provider == "woc" {
			node = newWOCNode(cfg)
		} else if cfg.Provider == "arc" {
			node = newARCNode(cfg)
		} else {
			rpc := NewRPCClient(cfg.RPCURL, cfg.RPCUser, cfg.RPCPass)
			node = newLiveNode(rpc, cfg)
		}
	default:
		rpc := NewRPCClient(cfg.RPCURL, cfg.RPCUser, cfg.RPCPass)
		node = &RegtestNode{rpc: rpc}
	}

	if !node.IsAvailable(context.Background()) {
		var msg string
		switch cfg.Provider {
		case "woc":
			msg = fmt.Sprintf("BSV %s WOC provider not available at %s", cfg.Network, cfg.WOCBaseURL)
		case "arc":
			target := cfg.ARCBaseURL
			if len(cfg.ARCBaseURLs) > 1 {
				target = strings.Join(cfg.ARCBaseURLs, ", ")
			}
			msg = fmt.Sprintf("BSV %s ARC provider not available at %s", cfg.Network, target)
		default:
			msg = fmt.Sprintf("BSV %s node not available at %s", cfg.Network, cfg.RPCURL)
		}
		if skipOnUnavailable {
			t.Skip(msg)
		}
		t.Fatal(msg)
	}
	return node
}

// --- TestNode interface implementation for RegtestNode ---

// Network returns "regtest".
func (n *RegtestNode) Network() string { return "regtest" }

// RPC returns the underlying RPCClient.
func (n *RegtestNode) RPC() *RPCClient { return n.rpc }

// IsAvailable returns true if the regtest node is reachable and responding.
func (n *RegtestNode) IsAvailable(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	var hash string
	err := n.rpc.Call(ctx, "getbestblockhash", nil, &hash)
	return err == nil && hash != ""
}

// Fund sends the specified amount to addr and returns a confirmed UTXO.
// On regtest, it mines 101 blocks (to make coinbase spendable), sends the
// amount, then mines 1 confirmation block.
func (n *RegtestNode) Fund(ctx context.Context, addr string, amount float64) (*UTXO, error) {
	miningAddr, err := n.NewAddress(ctx)
	if err != nil {
		return nil, fmt.Errorf("generate mining address: %w", err)
	}
	if _, err := n.MineBlocks(ctx, 101, miningAddr); err != nil {
		return nil, fmt.Errorf("mine 101 blocks: %w", err)
	}
	if _, err := n.SendToAddress(ctx, addr, amount); err != nil {
		return nil, fmt.Errorf("send to address: %w", err)
	}
	if _, err := n.MineBlocks(ctx, 1, miningAddr); err != nil {
		return nil, fmt.Errorf("mine confirmation: %w", err)
	}
	utxos, err := n.ListUnspent(ctx, addr)
	if err != nil {
		return nil, fmt.Errorf("list unspent: %w", err)
	}
	if len(utxos) == 0 {
		return nil, fmt.Errorf("no UTXOs found for %s", addr)
	}
	return &utxos[0], nil
}

// WaitForConfirmation on regtest simply mines the required number of blocks.
func (n *RegtestNode) WaitForConfirmation(ctx context.Context, txid string, minConf int) error {
	addr, err := n.NewAddress(ctx)
	if err != nil {
		return fmt.Errorf("generate mining address: %w", err)
	}
	_, err = n.MineBlocks(ctx, minConf, addr)
	return err
}

// --- RPC pass-through methods ---

// NewAddress generates a new address from the node's wallet.
func (n *RegtestNode) NewAddress(ctx context.Context) (string, error) {
	var addr string
	if err := n.rpc.Call(ctx, "getnewaddress", nil, &addr); err != nil {
		return "", fmt.Errorf("getnewaddress: %w", err)
	}
	return addr, nil
}

// MineBlocks mines the given number of blocks, sending coinbase rewards to addr.
// Returns the hashes of the generated blocks.
func (n *RegtestNode) MineBlocks(ctx context.Context, count int, addr string) ([]string, error) {
	var hashes []string
	params := []interface{}{count, addr}
	if err := n.rpc.Call(ctx, "generatetoaddress", params, &hashes); err != nil {
		return nil, fmt.Errorf("generatetoaddress(%d, %s): %w", count, addr, err)
	}
	return hashes, nil
}

// ListUnspent returns the unspent outputs for the given address.
// The address must be known to the node's wallet (imported or generated).
func (n *RegtestNode) ListUnspent(ctx context.Context, addr string) ([]UTXO, error) {
	var utxos []UTXO
	// listunspent minconf=1 maxconf=9999999 [addresses]
	params := []interface{}{1, 9999999, []string{addr}}
	if err := n.rpc.Call(ctx, "listunspent", params, &utxos); err != nil {
		return nil, fmt.Errorf("listunspent(%s): %w", addr, err)
	}
	return utxos, nil
}

// SendRawTransaction broadcasts a signed raw transaction hex to the network.
// Returns the transaction ID.
func (n *RegtestNode) SendRawTransaction(ctx context.Context, rawTxHex string) (string, error) {
	var txid string
	params := []interface{}{rawTxHex}
	if err := n.rpc.Call(ctx, "sendrawtransaction", params, &txid); err != nil {
		return "", fmt.Errorf("sendrawtransaction: %w", err)
	}
	return txid, nil
}

// SendToAddress sends the given BTC amount to addr from the node's wallet.
// Returns the transaction ID.
func (n *RegtestNode) SendToAddress(ctx context.Context, addr string, amount float64) (string, error) {
	var txid string
	params := []interface{}{addr, amount}
	if err := n.rpc.Call(ctx, "sendtoaddress", params, &txid); err != nil {
		return "", fmt.Errorf("sendtoaddress(%s, %f): %w", addr, amount, err)
	}
	return txid, nil
}

// GetRawTransaction returns the raw transaction bytes for the given txid.
func (n *RegtestNode) GetRawTransaction(ctx context.Context, txid string) ([]byte, error) {
	var rawHex string
	// getrawtransaction txid verbose=false
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
func (n *RegtestNode) GetTxStatus(ctx context.Context, txid string) (*TxStatus, error) {
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
func (n *RegtestNode) GetBlockHeader(ctx context.Context, blockHash string) ([]byte, error) {
	var headerHex string
	// getblockheader hash verbose=false -> returns hex string of 80-byte header
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
func (n *RegtestNode) GetBlockHeaderVerbose(ctx context.Context, blockHash string) (map[string]interface{}, error) {
	var result map[string]interface{}
	// getblockheader hash verbose=true
	params := []interface{}{blockHash, true}
	if err := n.rpc.Call(ctx, "getblockheader", params, &result); err != nil {
		return nil, fmt.Errorf("getblockheader verbose(%s): %w", blockHash, err)
	}
	return result, nil
}

// GetBlockTxIDs returns all txids in the block (display hex).
func (n *RegtestNode) GetBlockTxIDs(ctx context.Context, blockHash string) ([]string, error) {
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
// The transaction must be in the UTXO set or in the mempool.
func (n *RegtestNode) GetTxOutProof(ctx context.Context, txid string) ([]byte, error) {
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
func (n *RegtestNode) GetMerkleProof(ctx context.Context, txid string) (*MerkleProof, error) {
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
func (n *RegtestNode) GetBestBlockHash(ctx context.Context) (string, error) {
	var hash string
	if err := n.rpc.Call(ctx, "getbestblockhash", nil, &hash); err != nil {
		return "", fmt.Errorf("getbestblockhash: %w", err)
	}
	return hash, nil
}

// GetBlockHash returns the block hash at the given height.
func (n *RegtestNode) GetBlockHash(ctx context.Context, height int) (string, error) {
	var hash string
	params := []interface{}{height}
	if err := n.rpc.Call(ctx, "getblockhash", params, &hash); err != nil {
		return "", fmt.Errorf("getblockhash(%d): %w", height, err)
	}
	return hash, nil
}

// GetBlockCount returns the current block height.
func (n *RegtestNode) GetBlockCount(ctx context.Context) (int64, error) {
	var count int64
	if err := n.rpc.Call(ctx, "getblockcount", nil, &count); err != nil {
		return 0, fmt.Errorf("getblockcount: %w", err)
	}
	return count, nil
}

// ImportAddress imports an address (or script) for watch-only tracking.
// This allows ListUnspent to find UTXOs sent to non-wallet addresses.
func (n *RegtestNode) ImportAddress(ctx context.Context, addr string) error {
	// importaddress address label="" rescan=false
	params := []interface{}{addr, "", false}
	if err := n.rpc.Call(ctx, "importaddress", params, nil); err != nil {
		return fmt.Errorf("importaddress(%s): %w", addr, err)
	}
	return nil
}
