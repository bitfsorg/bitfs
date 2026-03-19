# Known Limitations — v0.0.1

## Release Constraints

- **Source install only** — no pre-compiled binaries; requires Go >= 1.25
- **CLI only** — no browser extension or mobile app in this release

## Network

- **Mainnet requires own RPC node** — CLI connects to BSV via JSON-RPC only; no built-in WoC/ARC REST API support. Mainnet users must provide `--rpc-url` pointing to their own BSV node or a hosted RPC service. Regtest and testnet presets connect to localhost by default.

## Design Limitations

- **Write permission is application-layer enforced** — not cryptographically enforced via covenant; planned for v0.0.2+
- **share_list field parsed but not enforced** — parser accepts the field for forward compatibility; access control uses acl_ref and access mode only
- **Pricing unit is sat/KB in code** — design docs reference sat/byte; will be aligned in a future release

## Naming

- The Metanet CDN network will be renamed to Penglai in a future release; current code and docs use the Metanet name
