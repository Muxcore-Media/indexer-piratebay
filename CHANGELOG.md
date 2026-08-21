# Changelog

## v0.1.1 (2026-08-20)

- Mesh StorageService cache: search hits with a btih are written to `torrent/{infohash}/index.json` when `MUXCORE_GRPC_ADDR` is set (same key space as downloader pieces / metainfo).

## Unreleased

- Live Apibay HTTP source-bound to WireGuard iface from `WG_CONF` (`SO_BINDTODEVICE`); refuse live base without VPN
- `INDEXER_FIXTURE=1` embedded search fixtures for offline caps/search; capability discovery without live Apibay
- Movie searches append `year` to the Apibay query (vault-tested)

## v0.1.2 (2026-08-09)

- Optional torrent detail enrichment via Apibay `/t.php` (`PIRATEBAY_ENRICH_DETAILS=1`, settings `enrich_details` / `enrich_limit`; off by default). Refreshes seeders/peers/size/imdb and points `info_url` at `/t.php` when a description is present.

## v0.1.0

- Initial release: `IndexerService` for The Pirate Bay via Apibay-compatible JSON
- Capability `indexer` (+ `settings`) for media-automation fan-out and runtime base URL
- Live HTTP only when base URL is set (`PIRATEBAY_API_BASE` or admin `base_url`); CI uses fixtures/`httptest` only
- Polite client rate limiting and retry/backoff on upstream errors
- Broader Apibay category mapping for search types and common category aliases
