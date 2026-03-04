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

func TestSatPerKBFromMiningFee(t *testing.T) {
	tests := []struct {
		name     string
		satoshis uint64
		bytes    uint64
		want     uint64
		wantErr  bool
	}{
		{name: "standard 100/1000", satoshis: 100, bytes: 1000, want: 100},
		{name: "half sat per byte", satoshis: 1, bytes: 2, want: 500},
		{name: "ceil division", satoshis: 1, bytes: 3, want: 334},
		{name: "bytes zero", satoshis: 100, bytes: 0, wantErr: true},
		{name: "satoshis zero", satoshis: 0, bytes: 1000, wantErr: true},
		{name: "min clamp", satoshis: 1, bytes: 10000000, want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := satPerKBFromMiningFee(tt.satoshis, tt.bytes)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParsePolicyResponse_Validation(t *testing.T) {
	_, err := parsePolicyResponse(nil)
	require.Error(t, err)

	_, err = parsePolicyResponse(&arcPolicyResponse{})
	require.Error(t, err)

	_, err = parsePolicyResponse(&arcPolicyResponse{
		Policy: &arcPolicy{
			MiningFee: &arcMiningFee{},
		},
	})
	require.Error(t, err)
}

func TestResolveFeeRate_OverrideSkipsPolicyFetch(t *testing.T) {
	var policyHits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/policy" {
			policyHits.Add(1)
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := newARCClient("testnet", []string{srv.URL}, "")
	cfg := &Config{Network: "testnet", FeeRateSatPerKB: 250}

	resolution, err := c.resolveFeeRate(context.Background(), cfg, false)
	require.NoError(t, err)
	assert.Equal(t, uint64(250), resolution.FeeRateSatPerKB)
	assert.Equal(t, feeSourceOverride, resolution.Source)
	assert.Equal(t, int32(0), policyHits.Load())
}

func TestResolveFeeRate_CacheTTLAndRefresh(t *testing.T) {
	var policyHits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/policy", r.URL.Path)
		policyHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"policy":{"miningFee":{"satoshis":1,"bytes":2}}}`))
	}))
	defer srv.Close()

	c := newARCClient("testnet", []string{srv.URL}, "")
	now := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return now }

	cfg := &Config{Network: "testnet"}

	first, err := c.resolveFeeRate(context.Background(), cfg, false)
	require.NoError(t, err)
	assert.Equal(t, uint64(500), first.FeeRateSatPerKB)
	assert.Equal(t, feeSourcePolicy, first.Source)
	assert.Equal(t, int32(1), policyHits.Load())

	second, err := c.resolveFeeRate(context.Background(), cfg, false)
	require.NoError(t, err)
	assert.Equal(t, uint64(500), second.FeeRateSatPerKB)
	assert.Equal(t, int32(1), policyHits.Load(), "cache hit should not fetch policy")

	now = now.Add(policyCacheTTL + time.Second)
	third, err := c.resolveFeeRate(context.Background(), cfg, false)
	require.NoError(t, err)
	assert.Equal(t, uint64(500), third.FeeRateSatPerKB)
	assert.Equal(t, int32(2), policyHits.Load(), "expired cache should fetch policy")

	fourth, err := c.resolveFeeRate(context.Background(), cfg, true)
	require.NoError(t, err)
	assert.Equal(t, uint64(500), fourth.FeeRateSatPerKB)
	assert.Equal(t, int32(3), policyHits.Load(), "refresh=true should bypass cache")
}

func TestResolveFeeRate_EndpointScopedCache(t *testing.T) {
	var aHits atomic.Int32
	a := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/policy", r.URL.Path)
		aHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"policy":{"miningFee":{"satoshis":100,"bytes":1000}}}`))
	}))
	defer a.Close()

	var bHits atomic.Int32
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/policy", r.URL.Path)
		bHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"policy":{"miningFee":{"satoshis":1,"bytes":1}}}`))
	}))
	defer b.Close()

	c := newARCClient("mainnet", []string{a.URL, b.URL}, "")
	cfg := &Config{Network: "mainnet"}

	first, err := c.resolveFeeRate(context.Background(), cfg, false)
	require.NoError(t, err)
	assert.Equal(t, uint64(100), first.FeeRateSatPerKB)
	assert.Equal(t, int32(1), aHits.Load())
	assert.Equal(t, int32(0), bHits.Load())

	// Force endpoint switch: cache lookup should be scoped per endpoint.
	c.markActive(b.URL)
	second, err := c.resolveFeeRate(context.Background(), cfg, false)
	require.NoError(t, err)
	assert.Equal(t, uint64(1000), second.FeeRateSatPerKB)
	assert.Equal(t, int32(1), aHits.Load())
	assert.Equal(t, int32(1), bHits.Load())
}

func TestResolveFeeRate_PolicyFailureFallsBack(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/policy", r.URL.Path)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer srv.Close()

	c := newARCClient("testnet", []string{srv.URL}, "")
	cfg := &Config{Network: "testnet"}

	resolution, err := c.resolveFeeRate(context.Background(), cfg, false)
	require.Error(t, err)
	assert.Equal(t, fallbackFeeRateSatPerKB, resolution.FeeRateSatPerKB)
	assert.Equal(t, feeSourceFallback, resolution.Source)
}
