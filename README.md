# Indexer Pirate Bay

[![Go Version](https://img.shields.io/badge/Go-1.26-blue)](https://go.dev/)
[![License: GPL-3.0](https://img.shields.io/badge/License-GPL--3.0-blue.svg)](LICENSE)

**Single-site MuxCore indexer for The Pirate Bay**, implementing `contracts-indexer` `IndexerService` via an Apibay-compatible JSON search API.

Advertises capabilities `indexer`, `indexer.piratebay`, and `settings` so `media-automation` can discover and fan out searches, and operators can set the API base at runtime.

---

## Network safety

- **CI and unit tests never call public Pirate Bay / Apibay mirrors.** They use hand-written fixtures and `httptest`.
- Offline smoke: set `INDEXER_FIXTURE=1` (host runner clears `PIRATEBAY_API_BASE`). Search returns embedded sample hits; no HTTP.
- Live HTTP is used **only** when an Apibay-compatible base URL is configured (`PIRATEBAY_API_BASE` or admin setting `base_url`) **and** a readable `WG_CONF` is present.
- Live HTTP is **source-bound** to the WireGuard interface derived from `WG_CONF` (same policy as `downloader-native-torrent`: `SO_BINDTODEVICE` + tunnel `LocalAddr`). There is no clearnet fallback.
- With an empty base URL and fixture off, `Search` returns no results and `ListIndexers` reports `configured=false`. `GetCapabilities` / module `indexer` capability still work for admin and automation discovery.

Operators should configure a base URL only when Proton WireGuard is up (`./scripts/check-vpn-up.sh` on gringotts). Example shape (not used by CI): `https://apibay.org`.

---

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `PIRATEBAY_GRPC_ADDR` | `:9485` | gRPC listen address (registered as `HTTPAddr` for discovery) |
| `PIRATEBAY_API_BASE` | _(empty)_ | Apibay-compatible API base; live search requires this **and** `WG_CONF` |
| `INDEXER_FIXTURE` | `0` | When `1`/`true`, serve embedded search fixtures (no network; ignores live base) |
| `WG_CONF` | _(empty)_ | WireGuard conf path; required for live Apibay HTTP (iface = conf basename) |
| `PIRATEBAY_ENRICH_DETAILS` | `false` | When `1`/`true`, fetch `/t.php` for top search hits (extra upstream calls) |
| `PIRATEBAY_ENRICH_LIMIT` | `5` | Max hits to enrich (capped at 25) |
| `PIRATEBAY_MIN_INTERVAL` | `2s` | Minimum interval between Apibay HTTP requests |
| `PIRATEBAY_CACHE_TTL` | `10m` | In-memory search result cache TTL |
| `MUXCORE_GRPC_ADDR` | — | Core mesh address |
| `MUXCORE_INSECURE_DISABLE_TLS` | `false` | Disable TLS for local core |

### Admin settings

| Key | Description |
|-----|-------------|
| `base_url` | Apibay-compatible API base (same role as `PIRATEBAY_API_BASE`; updates the live client) |
| `enrich_details` | Opt-in detail enrichment (default off) |
| `enrich_limit` | How many paged hits to enrich via `/t.php` |

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

- `Search` — query Apibay `/q.php` when configured; maps hits to magnets (`info_hash`); rate-limited with retry/backoff. Optional `/t.php` enrichment refreshes seeders/size/imdb (off by default).
- `GetCapabilities` — torrent search across movie/tv/audio/ebook/app/game and related categories
- `ListIndexers` — single entry `The Pirate Bay`

---

## Development

```bash
make test   # fixtures + httptest only
make build
```
