//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/bitfsorg/bitfs/e2e/testutil"
	"github.com/bitfsorg/libbitfs-go/spv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSPVVerify_RegtestMerkleProof performs a full SPV verification round-trip
// using the active TestNode provider:
//  1. Create a confirmed funding transaction
//  2. Fetch raw tx bytes + normalized merkle proof from the provider
//  3. Load the containing block header
//  4. Verify proof/root consistency and run spv.VerifyTransaction
//  5. Tamper with proof and ensure verification fails
func TestSPVVerify_RegtestMerkleProof(t *testing.T) {
	node := testutil.NewRequiredTestNode(t)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.NetworkAwareTestTimeout(10*time.Minute))
	defer cancel()

	txid, rawTx, proof, blockHeader := buildSPVFixtureFromNode(t, ctx, node)

	// Build SPV proof object.
	spvProof := &spv.MerkleProof{
		TxID:      proof.TxID,
		Index:     proof.Index,
		Nodes:     proof.Nodes,
		BlockHash: blockHeader.Hash,
	}

	// Sanity check: merkle root from proof must match block header root.
	computedRoot := spv.ComputeMerkleRoot(spvProof.TxID, spvProof.Index, spvProof.Nodes)
	require.NotNil(t, computedRoot, "computed merkle root should not be nil")
	require.Equal(t, blockHeader.MerkleRoot, computedRoot, "proof root should match block header merkle root")

	headerStore := spv.NewMemHeaderStore()
	require.NoError(t, headerStore.PutHeader(blockHeader), "store block header")

	storedTx := &spv.StoredTx{
		TxID:        spvProof.TxID,
		RawTx:       rawTx,
		Proof:       spvProof,
		BlockHeight: blockHeader.Height,
		Timestamp:   uint64(blockHeader.Timestamp),
	}

	// Verify should pass.
	err := spv.VerifyTransaction(storedTx, headerStore)
	assert.NoError(t, err, "VerifyTransaction should succeed with valid proof (txid=%s)", txid)

	// Tamper with first proof node and verify should fail.
	tamperedNodes := make([][]byte, len(spvProof.Nodes))
	for i, n := range spvProof.Nodes {
		c := make([]byte, len(n))
		copy(c, n)
		tamperedNodes[i] = c
	}
	if len(tamperedNodes) > 0 {
		tamperedNodes[0][0] ^= 0xFF
	}
	tamperedProofTxID := make([]byte, len(spvProof.TxID))
	copy(tamperedProofTxID, spvProof.TxID)
	if len(tamperedProofTxID) > 0 {
		tamperedProofTxID[0] ^= 0xFF
	}

	tamperedTx := &spv.StoredTx{
		TxID:        spvProof.TxID,
		RawTx:       rawTx,
		Proof:       &spv.MerkleProof{TxID: tamperedProofTxID, Index: spvProof.Index, Nodes: tamperedNodes, BlockHash: spvProof.BlockHash},
		BlockHeight: blockHeader.Height,
		Timestamp:   uint64(blockHeader.Timestamp),
	}
	err = spv.VerifyTransaction(tamperedTx, headerStore)
	assert.Error(t, err, "VerifyTransaction should fail with tampered proof")
	assert.ErrorIs(t, err, spv.ErrMerkleProofInvalid, "error should be ErrMerkleProofInvalid")
}

// TestSPVVerify_ManualMerkleBranch cross-checks SPV verification by rebuilding
// a merkle branch from full block txids, then verifying through the same SPV pipeline.
func TestSPVVerify_ManualMerkleBranch(t *testing.T) {
	node := testutil.NewRequiredTestNode(t)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.NetworkAwareTestTimeout(10*time.Minute))
	defer cancel()

	txid, rawTx, baseProof, blockHeader := buildSPVFixtureFromNode(t, ctx, node)

	status, err := node.GetTxStatus(ctx, txid)
	if err != nil || status == nil || status.BlockHash == "" {
		t.Logf("tx has no confirmed block hash on %s; fallback to fixture proof branch: %v", node.Network(), err)
		verifySPVWithProof(t, rawTx, baseProof, blockHeader, "fixture proof")
		return
	}

	txIDs, err := node.GetBlockTxIDs(ctx, status.BlockHash)
	if err != nil || len(txIDs) == 0 {
		t.Logf("provider block tx list unavailable, fallback to fixture proof branch: %v", err)
		verifySPVWithProof(t, rawTx, baseProof, blockHeader, "fallback fixture proof")
		return
	}

	// Convert txids to internal byte order and locate the target tx index.
	txHashes := make([][]byte, len(txIDs))
	targetIndex := -1
	for i, id := range txIDs {
		txIDBytes, err := hex.DecodeString(id)
		require.NoError(t, err, "decode txid %s", id)
		reverseBytes(txIDBytes)
		txHashes[i] = txIDBytes
		if id == txid {
			targetIndex = i
		}
	}
	require.NotEqual(t, -1, targetIndex, "target tx should exist in block tx list")

	computedTxID := spv.DoubleHash(rawTx)
	require.True(t, bytes.Equal(txHashes[targetIndex], computedTxID),
		"txid from block should match computed txid at index %d", targetIndex)

	branchNodes := buildMerkleBranch(txHashes, uint32(targetIndex))
	computedRoot := spv.ComputeMerkleRoot(computedTxID, uint32(targetIndex), branchNodes)
	require.NotNil(t, computedRoot)
	require.Equal(t, blockHeader.MerkleRoot, computedRoot,
		"manual branch merkle root should match block header")

	headerStore := spv.NewMemHeaderStore()
	require.NoError(t, headerStore.PutHeader(blockHeader))

	storedTx := &spv.StoredTx{
		TxID:  computedTxID,
		RawTx: rawTx,
		Proof: &spv.MerkleProof{
			TxID:      computedTxID,
			Index:     uint32(targetIndex),
			Nodes:     branchNodes,
			BlockHash: blockHeader.Hash,
		},
		BlockHeight: blockHeader.Height,
		Timestamp:   uint64(blockHeader.Timestamp),
	}
	err = spv.VerifyTransaction(storedTx, headerStore)
	assert.NoError(t, err, "VerifyTransaction should succeed with manually built proof")
}

func buildSPVFixtureFromNode(t *testing.T, ctx context.Context, node testutil.TestNode) (txid string, rawTx []byte, proof *testutil.MerkleProof, header *spv.BlockHeader) {
	t.Helper()
	fundAmount := testutil.LoadConfig().FundAmount

	addr, err := node.NewAddress(ctx)
	require.NoError(t, err, "generate destination address")

	utxo, err := node.Fund(ctx, addr, fundAmount)
	require.NoError(t, err, "fund destination address")
	require.NotEmpty(t, utxo.TxID, "funding txid should not be empty")

	rawTx, err = node.GetRawTransaction(ctx, utxo.TxID)
	require.NoError(t, err, "get raw transaction")
	require.NotEmpty(t, rawTx)

	computedTxID := spv.DoubleHash(rawTx)

	proofCtx := ctx
	cancelProof := func() {}
	if node.Network() != "regtest" {
		proofCtx, cancelProof = context.WithTimeout(ctx, 15*time.Second)
	}
	defer cancelProof()

	proof, err = node.GetMerkleProof(proofCtx, utxo.TxID)
	if err != nil {
		require.NotEqual(t, "regtest", node.Network(), "regtest must provide chain merkle proof")
		t.Logf("provider merkle proof unavailable on %s without confirmation; using synthetic proof/header: %v", node.Network(), err)
		proof, header, err = buildSyntheticSPVFixture(rawTx)
		require.NoError(t, err, "build synthetic SPV fixture")
		return utxo.TxID, rawTx, proof, header
	}
	require.NotEmpty(t, proof.BlockHash, "proof must include block hash")

	require.Equal(t, computedTxID, proof.TxID, "proof txid should match computed txid")

	headerBytes, err := node.GetBlockHeader(ctx, proof.BlockHash)
	require.NoError(t, err, "get block header")

	blockHeader, err := spv.DeserializeHeader(headerBytes)
	require.NoError(t, err, "deserialize block header")

	verbose, err := node.GetBlockHeaderVerbose(ctx, proof.BlockHash)
	require.NoError(t, err, "get verbose block header")
	height, ok := verbose["height"].(float64)
	require.True(t, ok, "verbose block header should include numeric height")
	blockHeader.Height = uint32(height)

	return utxo.TxID, rawTx, proof, blockHeader
}

func verifySPVWithProof(t *testing.T, rawTx []byte, proof *testutil.MerkleProof, blockHeader *spv.BlockHeader, label string) {
	t.Helper()
	require.NotNil(t, proof, "%s: proof should not be nil", label)
	require.NotNil(t, blockHeader, "%s: block header should not be nil", label)

	computedTxID := spv.DoubleHash(rawTx)
	require.Equal(t, computedTxID, proof.TxID, "%s: proof txid should match computed txid", label)

	computedRoot := spv.ComputeMerkleRoot(computedTxID, proof.Index, proof.Nodes)
	require.NotNil(t, computedRoot)
	require.Equal(t, blockHeader.MerkleRoot, computedRoot, "%s: proof merkle root should match block header", label)

	headerStore := spv.NewMemHeaderStore()
	require.NoError(t, headerStore.PutHeader(blockHeader))

	storedTx := &spv.StoredTx{
		TxID:  computedTxID,
		RawTx: rawTx,
		Proof: &spv.MerkleProof{
			TxID:      computedTxID,
			Index:     proof.Index,
			Nodes:     proof.Nodes,
			BlockHash: blockHeader.Hash,
		},
		BlockHeight: blockHeader.Height,
		Timestamp:   uint64(blockHeader.Timestamp),
	}
	err := spv.VerifyTransaction(storedTx, headerStore)
	assert.NoError(t, err, "VerifyTransaction should succeed with %s", label)
}

func buildSyntheticSPVFixture(rawTx []byte) (*testutil.MerkleProof, *spv.BlockHeader, error) {
	txid := spv.DoubleHash(rawTx)
	if len(txid) != 32 {
		return nil, nil, fmt.Errorf("invalid txid length: %d", len(txid))
	}

	header := &spv.BlockHeader{
		Version:    1,
		PrevBlock:  make([]byte, 32),
		MerkleRoot: append([]byte(nil), txid...),
		Timestamp:  uint32(time.Now().Unix()),
		Bits:       spv.RegtestMinBits,
		Nonce:      0,
		Height:     0,
	}

	if err := mineSyntheticHeader(header); err != nil {
		return nil, nil, err
	}

	proof := &testutil.MerkleProof{
		TxID:      append([]byte(nil), txid...),
		Index:     0,
		Nodes:     nil,
		BlockHash: internalHashToDisplayHex(header.Hash),
	}
	return proof, header, nil
}

func mineSyntheticHeader(h *spv.BlockHeader) error {
	if h == nil {
		return fmt.Errorf("nil header")
	}

	for {
		h.Hash = spv.ComputeHeaderHash(h)
		if err := spv.VerifyPoW(h); err == nil {
			return nil
		}
		h.Nonce++
		if h.Nonce == 0 {
			return fmt.Errorf("nonce overflow while mining synthetic header")
		}
	}
}

func internalHashToDisplayHex(hash []byte) string {
	if len(hash) == 0 {
		return ""
	}
	b := make([]byte, len(hash))
	copy(b, hash)
	reverseBytes(b)
	return hex.EncodeToString(b)
}

// reverseBytes reverses a byte slice in place.
func reverseBytes(b []byte) {
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
}

// buildMerkleBranch manually constructs a Merkle branch (proof nodes) for the
// transaction at targetIndex within the given list of transaction hashes.
func buildMerkleBranch(txHashes [][]byte, targetIndex uint32) [][]byte {
	if len(txHashes) <= 1 {
		return nil
	}

	// Work with a copy of the leaves.
	level := make([][]byte, len(txHashes))
	for i, h := range txHashes {
		level[i] = make([]byte, 32)
		copy(level[i], h)
	}

	var branch [][]byte
	idx := targetIndex

	for len(level) > 1 {
		// Pad if odd.
		if len(level)%2 == 1 {
			dup := make([]byte, 32)
			copy(dup, level[len(level)-1])
			level = append(level, dup)
		}

		// Sibling for current index.
		var sibling []byte
		if idx%2 == 0 {
			sibling = level[idx+1]
		} else {
			sibling = level[idx-1]
		}
		sibCopy := make([]byte, 32)
		copy(sibCopy, sibling)
		branch = append(branch, sibCopy)

		// Build parent level.
		next := make([][]byte, 0, len(level)/2)
		for i := 0; i < len(level); i += 2 {
			combined := make([]byte, 64)
			copy(combined[:32], level[i])
			copy(combined[32:], level[i+1])
			parent := spv.DoubleHash(combined)
			next = append(next, parent)
		}

		level = next
		idx = idx / 2
	}

	return branch
}
