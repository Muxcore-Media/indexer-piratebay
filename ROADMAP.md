# Roadmap

## Done

- [x] `IndexerService` against Apibay-compatible JSON
- [x] Soft-fail when `PIRATEBAY_API_BASE` unset (no outbound HTTP)
- [x] Fixture / httptest tests

## Remaining

- [ ] Rate limiting / polite client backoff
- [ ] Richer category mapping (full Apibay category tree)
- [ ] Optional torrent detail enrichment (no extra calls by default)
- [ ] Admin settings / secrets-provider for base URL
