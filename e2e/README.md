# BitFS End-to-End Tests

End-to-end tests that exercise the full BitFS stack against a real BSV node.
Supports **regtest** (default), **testnet**, and **mainnet** networks.

## Prerequisites

- **Docker Desktop** installed and running (regtest only)
- **Go 1.25.6+**

## Quick Start (Regtest)

```bash
# Start the BSV regtest node
cd e2e && docker compose up -d

# Wait for node to be ready
docker compose exec bsv-node bitcoin-cli -regtest -rpcuser=bitfs -rpcpassword=bitfs getblockchaininfo

# Run all e2e tests
cd .. && go test -tags e2e ./e2e/... -v -timeout 600s

# Run specific test
go test -tags e2e ./e2e/ -run TestWalletFund -v

# Stop the node
cd e2e && docker compose down -v
```

## Multi-Network Testing

Tests are controlled via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `BITFS_E2E_NETWORK` | `regtest` | Network: `regtest`, `testnet`, `mainnet` |
| `BITFS_E2E_RPC_URL` | per network | RPC endpoint URL |
| `BITFS_E2E_RPC_USER` | `bitfs` | RPC username |
| `BITFS_E2E_RPC_PASS` | `bitfs` | RPC password |
| `BITFS_E2E_FAUCET_URL` | — | Testnet faucet API URL (optional) |
| `BITFS_E2E_FUND_WIF` | — | Pre-funded wallet WIF key |
| `BITFS_E2E_CONFIRM_TIMEOUT` | per network | Confirmation wait timeout |

### Network Defaults

| Network | RPC URL | Confirm Timeout | Funding |
|---------|---------|-----------------|---------|
| regtest | `http://localhost:18332` | 30s | Mining |
| testnet | `http://localhost:18333` | 30m | Faucet → WIF fallback |
| mainnet | `http://localhost:8332` | 60m | WIF only |

### Running on Testnet

```bash
# Start testnet node
cd e2e && docker compose -f docker-compose.testnet.yml up -d

# Run tests
BITFS_E2E_NETWORK=testnet go test -tags e2e ./e2e/... -v -timeout 60m
```

### Running on Mainnet

```bash
# Requires a pre-funded wallet (WIF private key)
BITFS_E2E_NETWORK=mainnet \
  BITFS_E2E_FUND_WIF=L... \
  go test -tags e2e ./e2e/... -v -timeout 120m
```

## Test Suite

### Core Tests (01-07)

| File | Description | Node |
|------|-------------|------|
| `01_wallet_fund_test.go` | HD wallet key derivation + funding | Yes |
| `02_metanet_root_test.go` | Metanet root directory tx broadcast | Yes |
| `03_mkdir_upload_test.go` | Directory + encrypted file upload chain | Yes |
| `04_spv_verify_test.go` | SPV Merkle proof verification | Yes |
| `05_free_content_test.go` | Free content via daemon HTTP | — |
| `06_paid_purchase_test.go` | x402 paid purchase with HTLC | Yes |
| `07_full_lifecycle_test.go` | Complete lifecycle smoke test | Yes |

### DAG Mutation Tests (08-11)

| File | Description | Node |
|------|-------------|------|
| `08_move_rename_test.go` | Move file between dirs + rename via SelfUpdate | Yes |
| `09_copy_test.go` | Copy file (new P_node, same content) + independence | Yes |
| `10_remove_test.go` | Remove file/dir from parent via SelfUpdate | Yes |
| `11_link_test.go` | Hard links (shared P_node) + soft links (target ref) | Yes |

### Access Control Tests (12-13)

| File | Description | Node |
|------|-------------|------|
| `12_encrypt_transition_test.go` | Free→Private transitions, access mode isolation | — |
| `13_sell_pricing_test.go` | Engine.Sell pricing + state updates | Yes |

### Vault & Wallet Tests (14-16)

| File | Description | Node |
|------|-------------|------|
| `14_vault_crud_test.go` | Vault create/list/rename/delete | — |
| `15_multi_vault_test.go` | Vault key isolation + encryption isolation | — |
| `16_fund_external_test.go` | External UTXO registration + use | Yes |

### Daemon HTTP API Tests (17-21)

| File | Description | Node |
|------|-------------|------|
| `17_daemon_handshake_test.go` | Method 42 ECDH handshake endpoint | — |
| `18_daemon_buy_api_test.go` | Buy API (GET info, POST HTLC, x402 headers) | — |
| `19_daemon_content_neg_test.go` | Content negotiation edge cases | — |
| `20_daemon_paymail_test.go` | BSV Alias capabilities + PKI lookup | — |
| `21_daemon_spv_endpoint_test.go` | SPV proof endpoint (mock SPVService) | — |

### Client & Integration Tests (22-25)

| File | Description | Node |
|------|-------------|------|
| `22_client_roundtrip_test.go` | Client library → daemon → response chain | — |
| `23_error_paths_test.go` | Double-spend, malformed tx, insufficient funds | Yes |
| `24_large_file_test.go` | 1MB encrypt/decrypt + daemon roundtrip | — |
| `25_self_update_chain_test.go` | Multiple sequential SelfUpdates, version chain | Yes |

## Troubleshooting

**Node won't start**
- Check that Docker Desktop is running.
- Verify ports 18332 (RPC) and 18444 (P2P) are not in use by another process.

**Coinbase maturity (regtest)**
- Regtest requires 100 confirmations before coinbase outputs are spendable.
  The test helpers mine 101 blocks to satisfy this requirement.

**Tests skip instead of running**
- If the node is unreachable, tests gracefully skip with a descriptive message.
- Regtest: start with `docker compose up -d`
- Testnet/mainnet: ensure RPC endpoint is reachable and credentials are correct.

**RPC authentication errors**
- The default credentials are `bitfs`/`bitfs`. Override with `BITFS_E2E_RPC_USER`
  and `BITFS_E2E_RPC_PASS` environment variables.

**Testnet/mainnet funding fails**
- Ensure `BITFS_E2E_FUND_WIF` contains a valid WIF private key with sufficient
  balance, or configure `BITFS_E2E_FAUCET_URL` for testnet.

## Architecture

```
e2e/
├── docker-compose.yml              BSV SV Node 1.0.11 (regtest)
├── docker-compose.testnet.yml      BSV SV Node (testnet)
├── bitcoin.conf                    Regtest config
├── bitcoin-testnet.conf            Testnet config
├── testutil/
│   ├── rpc.go                      JSON-RPC 1.0 client
│   ├── config.go                   Environment variable configuration
│   ├── types.go                    Shared types (UTXO)
│   ├── node.go                     TestNode interface + RegtestNode
│   ├── live_node.go                liveNode (testnet/mainnet)
│   ├── faucet.go                   Faucet funding adapter
│   ├── funder.go                   WIF wallet funding
│   └── engine_helpers.go           Vault + wallet setup helpers
└── *_test.go                       25 test files gated by `e2e` tag
```

- **TestNode interface** abstracts network differences. `RegtestNode` mines
  blocks for funding/confirmation; `liveNode` uses faucet/WIF funding and
  polls for confirmations.
- **Build tag `e2e`** gates all test files from regular `go test ./...` runs.
- Tests are **independent** but ordered by complexity.
