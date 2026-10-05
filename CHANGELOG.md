# Changelog

## [0.1.7] - 2026-10-05


### Changed

- Reported version comes from muxcore.json (ADR-0021); built on core v0.6.12 / sdk/go/module v0.6.3 (mesh enrollment, ADR-0017).

## [0.1.6] - 2026-10-05


### Fixed
- Fixture mode (`INDEXER_FIXTURE=1`) marks results (`IndexerName` "The Pirate Bay (fixture)", GUID prefixed `fixture-`) so media-automation's fixture-only dispatch guard (ADR-0008) accepts grabs. Live-mode output is unchanged.

## [0.1.5] - 2026-10-05

### Changed
- CI runs on GitHub-hosted runners from the umbrella template; retired-origin workflows removed.
- Dependencies resolve from published GitHub tags (no filesystem `replace`); requires core v0.6.0.

## v0.1.1 (2026-08-31)

- Live Apibay HTTP source-bound to WireGuard iface from `WG_CONF` (`SO_BINDTODEVICE`); refuse live base without VPN-bound client
- `INDEXER_FIXTURE=1` embedded search fixtures for offline caps/search; capability discovery without live Apibay
- Movie searches append `year` to the Apibay query; TV searches append `Sxx` / `SxxExx` / absolute tokens
- Mesh StorageService cache: search hits with a btih are written to `torrent/{infohash}/index.json` when core is reachable
- Optional torrent detail enrichment via Apibay `/t.php` (`PIRATEBAY_ENRICH_DETAILS=1`, settings `enrich_details` / `enrich_limit`; off by default)
- Search `Total` reports unpaged hit count; stable `User-Agent`; CI runs `golangci-lint`

## v0.1.0 (2026-08-09)

- Initial release: `IndexerService` for The Pirate Bay via Apibay-compatible JSON
- Capability `indexer` (+ `settings`) for media-automation fan-out and runtime base URL
- Live HTTP only when base URL is set (`PIRATEBAY_API_BASE` or admin `base_url`); CI uses fixtures/`httptest` only
- Polite client rate limiting and retry/backoff on upstream errors
- Broader Apibay category mapping for search types and common category aliases
