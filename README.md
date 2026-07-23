# Indexer Pirate Bay

[![Go Version](https://img.shields.io/badge/Go-1.26-blue)](https://go.dev/)
[![License: GPL-3.0](https://img.shields.io/badge/License-GPL--3.0-blue.svg)](LICENSE)

**Single-site MuxCore indexer for The Pirate Bay**, implementing `contracts-indexer` `IndexerService` via an Apibay-compatible JSON search API.

Advertises capabilities `indexer`, `indexer.piratebay`, and `settings` so `media-automation` can discover and fan out searches, and operators can set the API base at runtime.

---

## Network safety

- **CI and unit tests never call public Pirate Bay / Apibay mirrors.** They use hand-written fixtures and `httptest`.
- Live HTTP is used **only** when an Apibay-compatible base URL is configured (`PIRATEBAY_API_BASE` or admin setting `base_url`).
- With an empty base URL, `Search` returns no results and `ListIndexers` reports `configured=false`.

Operators should configure a base URL only when they have appropriate network access (e.g. VPN). Example shape (not used by CI): `https://apibay.org`.

---

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `PIRATEBAY_GRPC_ADDR` | `:9480` | gRPC listen address (registered as `HTTPAddr` for discovery) |
| `PIRATEBAY_API_BASE` | _(empty)_ | Apibay-compatible API base; required for live search |
| `MUXCORE_GRPC_ADDR` | — | Core mesh address |
| `MUXCORE_INSECURE_DISABLE_TLS` | `false` | Disable TLS for local core |

### Admin settings

| Key | Description |
|-----|-------------|
| `base_url` | Apibay-compatible API base (same role as `PIRATEBAY_API_BASE`; updates the live client) |

---

## Quick start

```bash
make build
export MUXCORE_INSECURE_DISABLE_TLS=true
# Leave PIRATEBAY_API_BASE unset until you intentionally enable a mirror:
./indexer-piratebay --muxcore-mesh-addr localhost:9090
```

---

## gRPC (`IndexerService`)

- `Search` — query Apibay `/q.php` when configured; maps hits to magnets (`info_hash`); rate-limited with retry/backoff
- `GetCapabilities` — torrent search across movie/tv/audio/ebook/app/game and related categories
- `ListIndexers` — single entry `The Pirate Bay`

---

## Development

```bash
make test   # fixtures + httptest only
make build
```
