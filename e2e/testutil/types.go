//go:build e2e

package testutil

// UTXO represents an unspent transaction output from a BSV node.
type UTXO struct {
	TxID          string  `json:"txid"`
	Vout          uint32  `json:"vout"`
	Address       string  `json:"address"`
	ScriptPubKey  string  `json:"scriptPubKey"`
	Amount        float64 `json:"amount"`
	Confirmations int     `json:"confirmations"`
}

// TxStatus represents confirmation state for a transaction.
type TxStatus struct {
	Confirmations int64  `json:"confirmations"`
	BlockHash     string `json:"blockhash"`
	BlockHeight   uint64 `json:"blockheight"`
}

// MerkleProof contains the normalized merkle proof needed by SPV verification.
// TxID and Nodes are in internal byte order (little-endian).
type MerkleProof struct {
	TxID      []byte
	Index     uint32
	Nodes     [][]byte
	BlockHash string // display hex
}
