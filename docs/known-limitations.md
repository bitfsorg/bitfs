# Known Limitations — v0.0.1

## Release Constraints

- **Source install only** — no pre-compiled binaries; requires Go >= 1.25
- **CLI only** — no browser extension or mobile app in this release

## Network

- **Regtest requires own RPC node** — Mainnet and testnet auto-connect via WoC (queries) + ARC (broadcast). Regtest still requires `--rpc-url` pointing to a local BSV node. Optional: set `BITFS_WOC_API_KEY` / `BITFS_ARC_API_KEY` for higher rate limits.

## Design Limitations

- **Write permission is application-layer enforced** — not cryptographically enforced via covenant; planned for v0.0.2+
- **share_list field parsed but not enforced** — parser accepts the field for forward compatibility; access control uses acl_ref and access mode only
- **ARC requires API key** — the default ARC endpoint (arc.taal.com) returns 401 without a key; broadcast falls back to WoC's `/tx/raw` endpoint which works without authentication

## Naming

- The Metanet CDN network will be renamed to Penglai in a future release; current code and docs use the Metanet name
