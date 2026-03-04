//go:build e2e

package testutil

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bsv-blockchain/go-sdk/chainhash"
)

type bhsHeader struct {
	Height        uint32         `json:"height"`
	Hash          chainhash.Hash `json:"hash"`
	Version       uint32         `json:"version"`
	MerkleRoot    chainhash.Hash `json:"merkleRoot"`
	Timestamp     uint32         `json:"creationTimestamp"`
	Bits          uint32         `json:"difficultyTarget"`
	Nonce         uint32         `json:"nonce"`
	PreviousBlock chainhash.Hash `json:"prevBlockHash"`
}

type bhsState struct {
	Header bhsHeader `json:"header"`
	State  string    `json:"state"`
	Height uint32    `json:"height"`
}

type bhsClient struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

func newBHSClient(baseURL, apiKey string) *bhsClient {
	return &bhsClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  strings.TrimSpace(apiKey),
		client: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

func (c *bhsClient) applyAuthHeaders(req *http.Request) {
	if c.apiKey == "" {
		return
	}
	token := c.apiKey
	req.Header.Set("Api-Key", token)
	req.Header.Set("X-Api-Key", token)
	if strings.HasPrefix(strings.ToLower(token), "bearer ") {
		req.Header.Set("Authorization", token)
	} else {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

func (c *bhsClient) getJSON(ctx context.Context, path string, out interface{}) error {
	for attempt := 0; attempt <= wocMaxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
		c.applyAuthHeaders(req)

		resp, err := c.client.Do(req)
		if err != nil {
			if attempt < wocMaxRetries {
				if sleepErr := sleepWithContext(ctx, retryDelay(attempt, "")); sleepErr == nil {
					continue
				}
			}
			return fmt.Errorf("GET %s: %w", path, err)
		}

		respBody, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			if attempt < wocMaxRetries {
				if sleepErr := sleepWithContext(ctx, retryDelay(attempt, resp.Header.Get("Retry-After"))); sleepErr == nil {
					continue
				}
			}
			return fmt.Errorf("read response body: %w", readErr)
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			if err := json.Unmarshal(respBody, out); err != nil {
				return fmt.Errorf("decode BHS response for %s: %w", path, err)
			}
			return nil
		}

		if isRetryableWOCStatus(resp.StatusCode) && attempt < wocMaxRetries {
			if sleepErr := sleepWithContext(ctx, retryDelay(attempt, resp.Header.Get("Retry-After"))); sleepErr == nil {
				continue
			}
		}
		return fmt.Errorf("BHS GET %s HTTP %d: %s", path, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return fmt.Errorf("BHS GET %s failed after retries", path)
}

func (c *bhsClient) tipLongest(ctx context.Context) (*bhsState, error) {
	var out bhsState
	if err := c.getJSON(ctx, "/api/v1/chain/tip/longest", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *bhsClient) stateByHash(ctx context.Context, hash string) (*bhsState, error) {
	var out bhsState
	if err := c.getJSON(ctx, "/api/v1/chain/header/state/"+hash, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *bhsClient) blockByHeight(ctx context.Context, height int64) (*bhsHeader, error) {
	var headers []bhsHeader
	if err := c.getJSON(ctx, fmt.Sprintf("/api/v1/chain/header/byHeight?height=%d", height), &headers); err != nil {
		return nil, err
	}
	if len(headers) == 0 {
		return nil, fmt.Errorf("no BHS header found at height %d", height)
	}

	for i := range headers {
		state, err := c.stateByHash(ctx, headers[i].Hash.String())
		if err != nil {
			continue
		}
		if strings.EqualFold(state.State, "LONGEST_CHAIN") {
			h := headers[i]
			h.Height = state.Height
			return &h, nil
		}
	}

	// Fallback when state endpoint is unavailable.
	h := headers[0]
	h.Height = uint32(height)
	return &h, nil
}

func encodeBHSHeader(h *bhsHeader) ([]byte, error) {
	if h == nil {
		return nil, fmt.Errorf("nil BHS header")
	}

	header := make([]byte, 80)
	binary.LittleEndian.PutUint32(header[0:4], h.Version)
	copy(header[4:36], h.PreviousBlock.CloneBytes())
	copy(header[36:68], h.MerkleRoot.CloneBytes())
	binary.LittleEndian.PutUint32(header[68:72], h.Timestamp)
	binary.LittleEndian.PutUint32(header[72:76], h.Bits)
	binary.LittleEndian.PutUint32(header[76:80], h.Nonce)
	return header, nil
}
