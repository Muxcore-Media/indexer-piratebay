# Changelog

## Unreleased

- Optional torrent detail enrichment via Apibay `/t.php` (`PIRATEBAY_ENRICH_DETAILS=1`, settings `enrich_details` / `enrich_limit`; off by default). Refreshes seeders/peers/size/imdb and points `info_url` at `/t.php` when a description is present.

## v0.1.0

- Initial release: `IndexerService` for The Pirate Bay via Apibay-compatible JSON
- Capability `indexer` (+ `settings`) for media-automation fan-out and runtime base URL
- Live HTTP only when base URL is set (`PIRATEBAY_API_BASE` or admin `base_url`); CI uses fixtures/`httptest` only
- Polite client rate limiting and retry/backoff on upstream errors
- Broader Apibay category mapping for search types and common category aliases
