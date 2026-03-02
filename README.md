# BitFS

Unix-style decentralized encrypted file system. Provides a CLI for file owners and a suite of read-only tools for visitors.

## Prerequisites

- Go >= 1.25
- [libbitfs-go](../libbitfs-go) (referenced via `replace` directive in go.mod)

## Quick Start

```bash
# Build
go build ./cmd/bitfs

# Initialize wallet
bitfs wallet init

# Start daemon
bitfs daemon

# Basic file operations
bitfs put myfile.txt /docs/
bitfs mkdir /projects
bitfs ls /
```

## Architecture

Three layers:

- **`cmd/bitfs/`** — Owner CLI (wallet, vault, put, mkdir, rm, mv, cp, link, sell, encrypt, publish, shell, daemon)
- **`cmd/b*/`** — Read-only visitor tools (bls, bcat, bget, bstat, btree, bmget), connect to daemon over HTTP
- **`internal/`** — Business logic
  - `daemon/` — LFCP HTTP server (content serving, Metanet metadata, Method 42, x402 payments)
  - `client/` — HTTP client for b-tools
  - `buy/` — Purchase flow for paid content
  - `publish/` — Content publishing pipeline

Core business logic lives in [libbitfs-go/vault](../libbitfs-go/vault/).

## Dashboard

`dashboard/` contains a React SPA embedded into the daemon at `/_dashboard/*`. See [dashboard/README.md](dashboard/README.md).

## Testing

```bash
# Unit tests
go test ./...

# Integration tests (276 tests, 19 files)
go test -tags=integration ./integration/ -count=1

# E2E tests (requires Docker Desktop)
cd e2e && docker compose up -d && cd ..
go test -tags=e2e ./e2e/... -v -timeout 120s
```

## License

OpenBSV License
