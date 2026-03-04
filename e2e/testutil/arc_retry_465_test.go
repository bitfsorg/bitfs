//go:build e2e

package testutil

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestARCNodeSendRawTransaction_RetryOnceOn465ThenSuccess(t *testing.T) {
	var txHits atomic.Int32
	var policyHits atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tx":
			txHits.Add(1)
			if txHits.Load() == 1 {
				w.WriteHeader(arcFeeTooLowStatusCode)
				_, _ = w.Write([]byte(`{"detail":"fee too low"}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":200,"txid":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","txStatus":"SEEN_ON_NETWORK"}`))
		case "/policy":
			policyHits.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"policy":{"miningFee":{"satoshis":1,"bytes":1}}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := &Config{
		Network:     "testnet",
		Provider:    "arc",
		ARCBaseURL:  srv.URL,
		ARCBaseURLs: []string{srv.URL},
	}
	node := newARCNode(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	txid, err := node.SendRawTransaction(ctx, "00")
	require.NoError(t, err)
	assert.Equal(t, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", txid)
	assert.Equal(t, int32(2), txHits.Load(), "should retry exactly once")
	assert.Equal(t, int32(1), policyHits.Load(), "should refresh policy on 465")
}

func TestARCNodeSendRawTransaction_RetryOnceOn465ThenFail(t *testing.T) {
	var txHits atomic.Int32
	var policyHits atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tx":
			txHits.Add(1)
			w.WriteHeader(arcFeeTooLowStatusCode)
			_, _ = w.Write([]byte(`{"detail":"fee too low"}`))
		case "/policy":
			policyHits.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"policy":{"miningFee":{"satoshis":2,"bytes":1}}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := &Config{
		Network:     "testnet",
		Provider:    "arc",
		ARCBaseURL:  srv.URL,
		ARCBaseURLs: []string{srv.URL},
	}
	node := newARCNode(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := node.SendRawTransaction(ctx, "00")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "retry_on_465=1")
	assert.Equal(t, int32(2), txHits.Load(), "should retry exactly once")
	assert.Equal(t, int32(1), policyHits.Load(), "should refresh policy once")
}
