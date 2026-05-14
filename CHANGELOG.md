# Changelog

## v0.0.2 — 2026-05-14

CLI polish release based on five rounds of Docker-based audits and a senior Unix user design review.

### Added

- `bitfs ls` top-level command (was shell-only / via bls)
- `bitfs status` overview command (network, wallet, vaults, daemon, storage)
- `--network` flag on all commands (was: implicit from wallet config)
- `BITFS_PASSWORD`, `BITFS_NETWORK`, `BITFS_DATADIR` environment variables
- `bitfs put -` reads from stdin (Unix pipe support)
- `--arc-url` flag for ARC backend customization
- BSV purchase links shown on insufficient funds
- Auto-broadcast for mainnet/testnet write commands (no manual broadcast step)
- WoC+ARC as default backend for mainnet/testnet (no RPC required)
- Invoice persistence (default-on): persist on create, cleanup on evict, recover unpaid

### Fixed

- Five rounds of CLI UX audit fixes: JSON consistency, exit codes, input validation,
  nested help, export safety, shell UX
- `verify` command auto-detects network from wallet config
- `publish` persists state immediately after adding binding
- `promptNetwork` out-of-bounds case removed

### Build

- Reference remote `libbitfs-go v0.0.2`; removed `replace` directive
- GoReleaser configuration for 7 binaries × 4 platforms (darwin/linux × amd64/arm64)
- `install.sh` one-line installer at `https://bitfs.org/install.sh`

## v0.0.1 — 2026-03-19

Initial release targeting early technical users.

### Features

- HD wallet with BIP44 derivation (m/44'/236'/account'/chain/index)
- Multi-vault filesystem management
- File operations: put, get, mkdir, rm, mv, cp, link, encrypt
- Method 42 encryption (AES-256-GCM, Private/Free/Paid access modes)
- Content selling via HTLC atomic swap (106-byte plain Bitcoin script)
- LFCP daemon with HTTP 402 payment protocol
- Paymail integration (bind/unbind/list)
- Read-only b-tools: bget, bcat, bls, bstat, btree, bmget
- Interactive shell (FTP-style REPL)
- SPV light client verification
- DNSLink domain publishing
- Embedded React dashboard
- Multi-network support (mainnet/testnet/regtest)
