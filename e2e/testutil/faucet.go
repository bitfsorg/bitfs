//go:build e2e

package testutil

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// faucetRequest is the JSON payload sent to a testnet faucet.
type faucetRequest struct {
	Address string  `json:"address"`
	Amount  float64 `json:"amount"`
}

// faucetFund requests testnet coins from the configured faucet URL.
// It POSTs a JSON body with the address and amount, and expects a 200 OK.
func faucetFund(ctx context.Context, faucetURL, addr string, amount float64) error {
	body, err := json.Marshal(faucetRequest{
		Address: addr,
		Amount:  amount,
	})
	if err != nil {
		return fmt.Errorf("marshal faucet request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, faucetURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create faucet request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("faucet HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("faucet returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}
