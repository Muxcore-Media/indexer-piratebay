# Changelog

## v0.1.0

- Initial release: `IndexerService` for The Pirate Bay via Apibay-compatible JSON
- Capability `indexer` (+ `settings`) for media-automation fan-out and runtime base URL
- Live HTTP only when base URL is set (`PIRATEBAY_API_BASE` or admin `base_url`); CI uses fixtures/`httptest` only
- Polite client rate limiting and retry/backoff on upstream errors
- Broader Apibay category mapping for search types and common category aliases
