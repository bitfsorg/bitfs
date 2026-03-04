//go:build e2e

package testutil

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	ec "github.com/bsv-blockchain/go-sdk/primitives/ec"
	"github.com/bsv-blockchain/go-sdk/script"
)

// wocNode implements TestNode over Whatsonchain REST APIs.
type wocNode struct {
	client *wocClient
	config *Config
}

var _ TestNode = (*wocNode)(nil)

func newWOCNode(cfg *Config) *wocNode {
	return &wocNode{
		client: newWOCClient(cfg.WOCBaseURL, cfg.WOCAPIKey),
		config: cfg,
	}
}

func (n *wocNode) Network() string { return n.config.Network }

func (n *wocNode) RPC() *RPCClient { return nil }

func (n *wocNode) IsAvailable(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	info, err := n.client.chainInfo(ctx)
	return err == nil && info.BestBlockHash != ""
}

func (n *wocNode) Fund(ctx context.Context, addr string, amount float64) (*UTXO, error) {
	if n.config.FundWIF == "" {
		return nil, fmt.Errorf("BITFS_E2E_FUND_WIF is required on %s", n.config.Network)
	}

	txid, rawHex, err := fundFromWIFWOC(ctx, n.client, n.config, n.config.FundWIF, addr, amount)
	if err != nil {
		return nil, fmt.Errorf("fund from WIF via WOC: %w", err)
	}

	if err := n.WaitForConfirmation(ctx, txid, 1); err != nil {
		return nil, fmt.Errorf("wait for funding propagation: %w", err)
	}

	utxo, parseErr := fundingUTXOFromRawTx(rawHex, addr, txid)
	if parseErr == nil {
		return utxo, nil
	}

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

func (n *wocNode) WaitForConfirmation(ctx context.Context, txid string, minConf int) error {
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
		}
	}
}

func (n *wocNode) MineBlocks(_ context.Context, _ int, _ string) ([]string, error) {
	return nil, fmt.Errorf("mining not supported on %s (WOC provider)", n.config.Network)
}

func (n *wocNode) NewAddress(_ context.Context) (string, error) {
	priv, err := ec.NewPrivateKey()
	if err != nil {
		return "", fmt.Errorf("generate private key: %w", err)
	}
	addr, err := script.NewAddressFromPublicKey(priv.PubKey(), n.config.IsMainnet())
	if err != nil {
		return "", fmt.Errorf("pubkey->address: %w", err)
	}
	return addr.AddressString, nil
}

func (n *wocNode) ListUnspent(ctx context.Context, addr string) ([]UTXO, error) {
	raw, err := n.client.listUnspent(ctx, addr)
	if err != nil {
		return nil, fmt.Errorf("WOC list unspent(%s): %w", addr, err)
	}

	best, err := n.GetBlockCount(ctx)
	if err != nil {
		best = 0 // confirmations can be left as 0 when chain info isn't available
	}

	utxos := make([]UTXO, 0, len(raw))
	for _, u := range raw {
		conf := 0
		if best > 0 && u.Height > 0 {
			conf = int(best-u.Height) + 1
			if conf < 0 {
				conf = 0
			}
		}
		utxos = append(utxos, UTXO{
			TxID:          u.TxHash,
			Vout:          u.TxPos,
			Address:       addr,
			ScriptPubKey:  "",
			Amount:        float64(u.Value) / 1e8,
			Confirmations: conf,
		})
	}
	return utxos, nil
}

func (n *wocNode) SendRawTransaction(ctx context.Context, rawTxHex string) (string, error) {
	txid, err := n.client.broadcastRawTx(ctx, rawTxHex)
	if err != nil {
		return "", fmt.Errorf("WOC broadcast: %w", err)
	}
	return txid, nil
}

func (n *wocNode) SendToAddress(ctx context.Context, addr string, amount float64) (string, error) {
	if n.config.FundWIF == "" {
		return "", fmt.Errorf("SendToAddress on WOC provider requires BITFS_E2E_FUND_WIF")
	}
	txid, _, err := fundFromWIFWOC(ctx, n.client, n.config, n.config.FundWIF, addr, amount)
	if err != nil {
		return "", fmt.Errorf("fund from WIF via WOC: %w", err)
	}
	return txid, nil
}

func (n *wocNode) GetRawTransaction(ctx context.Context, txid string) ([]byte, error) {
	rawHex, err := n.client.txHex(ctx, txid)
	if err != nil {
		return nil, fmt.Errorf("WOC tx hex(%s): %w", txid, err)
	}
	raw, err := hex.DecodeString(rawHex)
	if err != nil {
		return nil, fmt.Errorf("decode raw tx hex: %w", err)
	}
	return raw, nil
}

func (n *wocNode) GetTxStatus(ctx context.Context, txid string) (*TxStatus, error) {
	txInfo, err := n.client.txInfo(ctx, txid)
	if err != nil {
		return nil, fmt.Errorf("WOC tx status(%s): %w", txid, err)
	}
	return &TxStatus{
		Confirmations: txInfo.Confirmations,
		BlockHash:     txInfo.BlockHash,
		BlockHeight:   txInfo.BlockHeight,
	}, nil
}

func (n *wocNode) GetTxOutProof(ctx context.Context, txid string) ([]byte, error) {
	return nil, fmt.Errorf("gettxoutproof is not supported by WOC provider for tx %s", txid)
}

func (n *wocNode) GetMerkleProof(ctx context.Context, txid string) (*MerkleProof, error) {
	proof, err := n.client.tscProof(ctx, txid)
	if err != nil {
		return nil, fmt.Errorf("WOC tsc proof(%s): %w", txid, err)
	}

	txidDisplay := proof.TxOrID
	if !is64Hex(txidDisplay) {
		txidDisplay = txid
	}

	txIDInternal, err := decodeDisplayHashToInternal(txidDisplay)
	if err != nil {
		return nil, fmt.Errorf("decode proof txid: %w", err)
	}

	nodes := make([][]byte, 0, len(proof.Nodes))
	for _, nodeHex := range proof.Nodes {
		nodeHex = strings.TrimPrefix(nodeHex, "*")
		node, err := decodeDisplayHashToInternal(nodeHex)
		if err != nil {
			return nil, fmt.Errorf("decode proof node: %w", err)
		}
		nodes = append(nodes, node)
	}

	return &MerkleProof{
		TxID:      txIDInternal,
		Index:     proof.Index,
		Nodes:     nodes,
		BlockHash: proof.Target,
	}, nil
}

func (n *wocNode) GetBlockHeader(ctx context.Context, blockHash string) ([]byte, error) {
	info, err := n.client.blockByHash(ctx, blockHash)
	if err != nil {
		return nil, fmt.Errorf("WOC block(%s): %w", blockHash, err)
	}
	header, err := encodeWOCBlockHeader(info)
	if err != nil {
		return nil, err
	}
	return header, nil
}

func (n *wocNode) GetBlockHeaderVerbose(ctx context.Context, blockHash string) (map[string]interface{}, error) {
	info, err := n.client.blockByHash(ctx, blockHash)
	if err != nil {
		return nil, fmt.Errorf("WOC block(%s): %w", blockHash, err)
	}
	return map[string]interface{}{
		"hash":          info.Hash,
		"height":        float64(info.Height),
		"merkleroot":    info.MerkleRoot,
		"time":          float64(info.Time),
		"bits":          info.Bits,
		"nonce":         float64(info.Nonce),
		"confirmations": float64(info.Confirmations),
	}, nil
}

func (n *wocNode) GetBlockTxIDs(ctx context.Context, blockHash string) ([]string, error) {
	info, err := n.client.blockByHash(ctx, blockHash)
	if err != nil {
		return nil, fmt.Errorf("WOC block(%s): %w", blockHash, err)
	}
	return info.Tx, nil
}

func (n *wocNode) GetBestBlockHash(ctx context.Context) (string, error) {
	info, err := n.client.chainInfo(ctx)
	if err != nil {
		return "", fmt.Errorf("WOC chain info: %w", err)
	}
	return info.BestBlockHash, nil
}

func (n *wocNode) GetBlockHash(ctx context.Context, height int) (string, error) {
	info, err := n.client.blockByHeight(ctx, int64(height))
	if err != nil {
		return "", fmt.Errorf("WOC block by height(%d): %w", height, err)
	}
	return info.Hash, nil
}

func (n *wocNode) GetBlockCount(ctx context.Context) (int64, error) {
	info, err := n.client.chainInfo(ctx)
	if err != nil {
		return 0, fmt.Errorf("WOC chain info: %w", err)
	}
	return info.Blocks, nil
}

func (n *wocNode) ImportAddress(_ context.Context, _ string) error {
	// WoC is stateless for address lookups; importing addresses is unnecessary.
	return nil
}

func (n *wocNode) waitForUTXO(ctx context.Context, addr string) (*UTXO, error) {
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
		}
	}
}

func decodeDisplayHashToInternal(hashHex string) ([]byte, error) {
	b, err := hex.DecodeString(hashHex)
	if err != nil {
		return nil, err
	}
	if len(b) != 32 {
		return nil, fmt.Errorf("expected 32-byte hash, got %d", len(b))
	}
	reverseBytesInPlace(b)
	return b, nil
}

func encodeWOCBlockHeader(info *wocBlockInfo) ([]byte, error) {
	header := make([]byte, 80)

	binary.LittleEndian.PutUint32(header[0:4], uint32(info.Version))

	prev := make([]byte, 32)
	if info.PreviousBlockHash != "" {
		p, err := decodeDisplayHashToInternal(info.PreviousBlockHash)
		if err != nil {
			return nil, fmt.Errorf("decode prev block hash: %w", err)
		}
		copy(prev, p)
	}
	copy(header[4:36], prev)

	mr, err := decodeDisplayHashToInternal(info.MerkleRoot)
	if err != nil {
		return nil, fmt.Errorf("decode merkle root: %w", err)
	}
	copy(header[36:68], mr)

	binary.LittleEndian.PutUint32(header[68:72], uint32(info.Time))

	bits, err := strconv.ParseUint(info.Bits, 16, 32)
	if err != nil {
		return nil, fmt.Errorf("parse bits %q: %w", info.Bits, err)
	}
	binary.LittleEndian.PutUint32(header[72:76], uint32(bits))
	binary.LittleEndian.PutUint32(header[76:80], uint32(info.Nonce))

	return header, nil
}
