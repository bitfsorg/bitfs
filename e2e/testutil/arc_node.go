//go:build e2e

package testutil

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/bsv-blockchain/go-sdk/chainhash"
	ec "github.com/bsv-blockchain/go-sdk/primitives/ec"
	"github.com/bsv-blockchain/go-sdk/script"
	gosdktx "github.com/bsv-blockchain/go-sdk/transaction"
)

// arcNode implements TestNode over ARC APIs with optional BHS/WoC read fallbacks.
type arcNode struct {
	arc    *arcClient
	woc    *wocClient
	bhs    *bhsClient
	config *Config

	rawMu     sync.RWMutex
	rawByTxID map[string]string
}

var _ TestNode = (*arcNode)(nil)

func newARCNode(cfg *Config) *arcNode {
	var bhs *bhsClient
	if strings.TrimSpace(cfg.BHSBaseURL) != "" {
		bhs = newBHSClient(cfg.BHSBaseURL, cfg.BHSAPIKey)
	}
	return &arcNode{
		arc:       newARCClient(cfg.ARCBaseURLs, cfg.ARCAPIKey),
		woc:       newWOCClient(cfg.WOCBaseURL, cfg.WOCAPIKey),
		bhs:       bhs,
		config:    cfg,
		rawByTxID: make(map[string]string),
	}
}

func (n *arcNode) Network() string { return n.config.Network }

func (n *arcNode) RPC() *RPCClient { return nil }

func (n *arcNode) IsAvailable(ctx context.Context) bool {
	if len(n.config.ARCBaseURLs) == 0 {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	health, err := n.arc.health(ctx)
	if err != nil {
		return false
	}
	return health.Healthy
}

func (n *arcNode) Fund(ctx context.Context, addr string, amount float64) (*UTXO, error) {
	if n.config.FundWIF == "" {
		return nil, fmt.Errorf("BITFS_E2E_FUND_WIF is required on %s", n.config.Network)
	}

	txid, rawHex, err := fundFromWIFARC(ctx, n.arc, n.woc, n.config, n.config.FundWIF, addr, amount)
	if err != nil {
		return nil, fmt.Errorf("fund from WIF via ARC: %w", err)
	}
	n.cacheRawTx(txid, rawHex)

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

// WaitForConfirmation on ARC waits for transaction propagation, not mined confirmation.
// It returns once ARC knows the transaction (e.g. SEEN_ON_NETWORK or MINED).
func (n *arcNode) WaitForConfirmation(ctx context.Context, txid string, _ int) error {
	deadline := time.After(n.config.ConfirmTimeout)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		status, err := n.arc.txStatus(ctx, txid)
		if err == nil && isARCSeen(status) {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("timeout waiting for ARC tx propagation on %s after %v", txid, n.config.ConfirmTimeout)
		case <-ticker.C:
		}
	}
}

func (n *arcNode) MineBlocks(_ context.Context, _ int, _ string) ([]string, error) {
	return nil, fmt.Errorf("mining not supported on %s (ARC provider)", n.config.Network)
}

func (n *arcNode) NewAddress(_ context.Context) (string, error) {
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

func (n *arcNode) ListUnspent(ctx context.Context, addr string) ([]UTXO, error) {
	raw, err := n.woc.listUnspent(ctx, addr)
	if err != nil {
		return nil, fmt.Errorf("list unspent(%s): %w", addr, err)
	}

	best, err := n.GetBlockCount(ctx)
	if err != nil {
		best = 0
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

func (n *arcNode) SendRawTransaction(ctx context.Context, rawTxHex string) (string, error) {
	resp, err := n.arc.broadcastRawTx(ctx, rawTxHex, n.config)
	if err != nil {
		return "", fmt.Errorf("ARC broadcast: %w", err)
	}
	if isARCRejectedStatus(resp.txStatusValue()) {
		return "", fmt.Errorf("ARC broadcast rejected: status=%s title=%q detail=%q extra=%q",
			resp.txStatusValue(), resp.Title, arcStatusDetail(resp), arcStatusExtraInfo(resp))
	}
	txid := resp.txIDValue()
	if !is64Hex(txid) {
		return "", fmt.Errorf("ARC broadcast missing txid: status=%d title=%q", resp.Status, resp.Title)
	}
	n.cacheRawTx(txid, rawTxHex)
	return txid, nil
}

func (n *arcNode) SendToAddress(ctx context.Context, addr string, amount float64) (string, error) {
	if n.config.FundWIF == "" {
		return "", fmt.Errorf("SendToAddress on ARC provider requires BITFS_E2E_FUND_WIF")
	}
	txid, rawHex, err := fundFromWIFARC(ctx, n.arc, n.woc, n.config, n.config.FundWIF, addr, amount)
	if err != nil {
		return "", fmt.Errorf("fund from WIF via ARC: %w", err)
	}
	n.cacheRawTx(txid, rawHex)
	return txid, nil
}

func (n *arcNode) GetRawTransaction(ctx context.Context, txid string) ([]byte, error) {
	if rawHex, ok := n.getCachedRawTx(txid); ok {
		raw, err := hex.DecodeString(rawHex)
		if err != nil {
			return nil, fmt.Errorf("decode cached raw tx: %w", err)
		}
		return raw, nil
	}

	rawHex, err := n.woc.txHex(ctx, txid)
	if err != nil {
		return nil, fmt.Errorf("ARC raw tx fallback (WOC tx hex): %w", err)
	}
	raw, err := hex.DecodeString(rawHex)
	if err != nil {
		return nil, fmt.Errorf("decode raw tx hex: %w", err)
	}
	return raw, nil
}

func (n *arcNode) GetTxStatus(ctx context.Context, txid string) (*TxStatus, error) {
	st, err := n.arc.txStatus(ctx, txid)
	if err != nil {
		return nil, fmt.Errorf("ARC tx status(%s): %w", txid, err)
	}
	if isARCRejectedStatus(st.txStatusValue()) {
		return nil, fmt.Errorf("ARC tx %s rejected: status=%s detail=%q extra=%q",
			txid, st.txStatusValue(), arcStatusDetail(st), arcStatusExtraInfo(st))
	}

	confs := int64(0)
	if strings.EqualFold(st.txStatusValue(), "MINED") {
		confs = 1
	}
	return &TxStatus{
		Confirmations: confs,
		BlockHash:     st.blockHashValue(),
		BlockHeight:   st.blockHeightValue(),
	}, nil
}

func (n *arcNode) GetTxOutProof(_ context.Context, txid string) ([]byte, error) {
	return nil, fmt.Errorf("gettxoutproof is not supported by ARC provider for tx %s", txid)
}

func (n *arcNode) GetMerkleProof(ctx context.Context, txid string) (*MerkleProof, error) {
	deadline := time.After(n.config.ConfirmTimeout)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		st, err := n.arc.txStatus(ctx, txid)
		if err == nil && isARCMinedWithProof(st) {
			return arcStatusToMerkleProof(st, txid)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline:
			return nil, fmt.Errorf("timeout waiting for ARC merkle proof on %s after %v", txid, n.config.ConfirmTimeout)
		case <-ticker.C:
		}
	}
}

func (n *arcNode) GetBlockHeader(ctx context.Context, blockHash string) ([]byte, error) {
	if n.bhs != nil {
		state, err := n.bhs.stateByHash(ctx, blockHash)
		if err == nil {
			header, encErr := encodeBHSHeader(&state.Header)
			if encErr == nil {
				return header, nil
			}
		}
	}

	info, err := n.woc.blockByHash(ctx, blockHash)
	if err != nil {
		return nil, fmt.Errorf("get block header fallback(%s): %w", blockHash, err)
	}
	header, err := encodeWOCBlockHeader(info)
	if err != nil {
		return nil, err
	}
	return header, nil
}

func (n *arcNode) GetBlockHeaderVerbose(ctx context.Context, blockHash string) (map[string]interface{}, error) {
	if n.bhs != nil {
		state, err := n.bhs.stateByHash(ctx, blockHash)
		if err == nil {
			return map[string]interface{}{
				"hash":          state.Header.Hash.String(),
				"height":        float64(state.Height),
				"merkleroot":    state.Header.MerkleRoot.String(),
				"time":          float64(state.Header.Timestamp),
				"bits":          fmt.Sprintf("%08x", state.Header.Bits),
				"nonce":         float64(state.Header.Nonce),
				"confirmations": float64(0),
			}, nil
		}
	}

	info, err := n.woc.blockByHash(ctx, blockHash)
	if err != nil {
		return nil, fmt.Errorf("get block header verbose fallback(%s): %w", blockHash, err)
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

func (n *arcNode) GetBlockTxIDs(_ context.Context, _ string) ([]string, error) {
	return nil, fmt.Errorf("GetBlockTxIDs is not supported by ARC provider")
}

func (n *arcNode) GetBestBlockHash(ctx context.Context) (string, error) {
	if n.bhs != nil {
		tip, err := n.bhs.tipLongest(ctx)
		if err == nil {
			return tip.Header.Hash.String(), nil
		}
	}

	info, err := n.woc.chainInfo(ctx)
	if err != nil {
		return "", fmt.Errorf("get best block hash fallback: %w", err)
	}
	return info.BestBlockHash, nil
}

func (n *arcNode) GetBlockHash(ctx context.Context, height int) (string, error) {
	if n.bhs != nil {
		h, err := n.bhs.blockByHeight(ctx, int64(height))
		if err == nil {
			return h.Hash.String(), nil
		}
	}

	info, err := n.woc.blockByHeight(ctx, int64(height))
	if err != nil {
		return "", fmt.Errorf("get block hash fallback(%d): %w", height, err)
	}
	return info.Hash, nil
}

func (n *arcNode) GetBlockCount(ctx context.Context) (int64, error) {
	if n.bhs != nil {
		tip, err := n.bhs.tipLongest(ctx)
		if err == nil {
			return int64(tip.Height), nil
		}
	}

	info, err := n.woc.chainInfo(ctx)
	if err != nil {
		return 0, fmt.Errorf("get block count fallback: %w", err)
	}
	return info.Blocks, nil
}

func (n *arcNode) ImportAddress(_ context.Context, _ string) error {
	return nil
}

func (n *arcNode) waitForUTXO(ctx context.Context, addr string) (*UTXO, error) {
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

func (n *arcNode) cacheRawTx(txid, rawHex string) {
	if !is64Hex(txid) || strings.TrimSpace(rawHex) == "" {
		return
	}
	n.rawMu.Lock()
	n.rawByTxID[strings.ToLower(txid)] = strings.TrimSpace(rawHex)
	n.rawMu.Unlock()
}

func (n *arcNode) getCachedRawTx(txid string) (string, bool) {
	n.rawMu.RLock()
	raw, ok := n.rawByTxID[strings.ToLower(txid)]
	n.rawMu.RUnlock()
	return raw, ok
}

func isARCMinedWithProof(st *arcTxStatus) bool {
	if st == nil {
		return false
	}
	return strings.EqualFold(st.txStatusValue(), "MINED") &&
		st.blockHashValue() != "" &&
		st.merklePathValue() != ""
}

func isARCSeen(st *arcTxStatus) bool {
	if st == nil {
		return false
	}

	status := strings.ToUpper(strings.TrimSpace(st.txStatusValue()))
	if isARCRejectedStatus(status) {
		return false
	}
	switch status {
	case "NOT_FOUND", "UNKNOWN":
		return false
	}
	if status != "" {
		return true
	}

	if st.blockHashValue() != "" || st.merklePathValue() != "" {
		return true
	}
	return is64Hex(st.txIDValue())
}

func isARCRejectedStatus(status string) bool {
	s := strings.ToUpper(strings.TrimSpace(status))
	if s == "" {
		return false
	}
	return strings.Contains(s, "REJECT") || strings.Contains(s, "INVALID") || strings.Contains(s, "FAIL")
}

func arcStatusDetail(st *arcTxStatus) string {
	if st == nil || st.Detail == nil {
		return ""
	}
	return strings.TrimSpace(*st.Detail)
}

func arcStatusExtraInfo(st *arcTxStatus) string {
	if st == nil || st.ExtraInfo == nil {
		return ""
	}
	return strings.TrimSpace(*st.ExtraInfo)
}

func arcStatusToMerkleProof(st *arcTxStatus, txid string) (*MerkleProof, error) {
	if st == nil {
		return nil, fmt.Errorf("nil ARC status")
	}
	mpHex := st.merklePathValue()
	if mpHex == "" {
		return nil, fmt.Errorf("empty ARC merklePath for %s", txid)
	}

	mp, err := gosdktx.NewMerklePathFromHex(mpHex)
	if err != nil {
		return nil, fmt.Errorf("parse ARC merklePath: %w", err)
	}
	txHash, err := chainhash.NewHashFromHex(txid)
	if err != nil {
		return nil, fmt.Errorf("decode txid: %w", err)
	}
	proof, err := merklePathToProof(mp, txHash)
	if err != nil {
		return nil, err
	}
	proof.BlockHash = st.blockHashValue()
	return proof, nil
}

func merklePathToProof(mp *gosdktx.MerklePath, txHash *chainhash.Hash) (*MerkleProof, error) {
	if mp == nil || len(mp.Path) == 0 || len(mp.Path[0]) == 0 {
		return nil, fmt.Errorf("invalid merkle path")
	}

	indexedPath := make(gosdktx.IndexedPath, len(mp.Path))
	for h := 0; h < len(mp.Path); h++ {
		path := map[uint64]*gosdktx.PathElement{}
		for _, leaf := range mp.Path[h] {
			path[leaf.Offset] = leaf
		}
		indexedPath[h] = path
	}

	var txLeaf *gosdktx.PathElement
	for _, leaf := range mp.Path[0] {
		if leaf.Hash != nil && leaf.Hash.Equal(*txHash) {
			txLeaf = leaf
			break
		}
	}
	if txLeaf == nil {
		return nil, fmt.Errorf("txid %s not present in ARC merklePath", txHash.String())
	}

	nodes := make([][]byte, 0, len(mp.Path))
	working := txLeaf.Hash.CloneBytes()

	for height := 0; height < len(mp.Path); height++ {
		siblingOffset := (txLeaf.Offset >> uint(height)) ^ 1
		sibling := indexedPath.GetOffsetLeaf(height, siblingOffset)
		if sibling == nil {
			return nil, fmt.Errorf("missing sibling at height=%d offset=%d", height, siblingOffset)
		}

		workingHash, err := chainhash.NewHash(working)
		if err != nil {
			return nil, fmt.Errorf("build working hash: %w", err)
		}

		if sibling.Duplicate != nil && *sibling.Duplicate {
			node := make([]byte, len(working))
			copy(node, working)
			nodes = append(nodes, node)
			working = gosdktx.MerkleTreeParent(workingHash, workingHash).CloneBytes()
			continue
		}
		if sibling.Hash == nil {
			return nil, fmt.Errorf("missing sibling hash at height=%d offset=%d", height, siblingOffset)
		}

		siblingBytes := sibling.Hash.CloneBytes()
		nodes = append(nodes, siblingBytes)

		if (siblingOffset % 2) != 0 {
			working = gosdktx.MerkleTreeParent(workingHash, sibling.Hash).CloneBytes()
		} else {
			working = gosdktx.MerkleTreeParent(sibling.Hash, workingHash).CloneBytes()
		}
	}

	return &MerkleProof{
		TxID:  txHash.CloneBytes(),
		Index: uint32(txLeaf.Offset),
		Nodes: nodes,
	}, nil
}
