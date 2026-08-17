package internal

import (
	"context"
	_ "embed"
	"strings"
)

//go:embed testdata/search_sample.json
var fixtureSearchJSON []byte

// fixtureAPI serves offline Apibay-shaped hits (INDEXER_FIXTURE=1). No network.
type fixtureAPI struct {
	hits []apibayHit
}

func newFixtureAPI() *fixtureAPI {
	hits, err := parseApibayHits(fixtureSearchJSON)
	if err != nil || len(hits) == 0 {
		hits = []apibayHit{
			{
				ID: "1", Name: "Fight.Club.1999.1080p.BluRay.x264",
				InfoHash: "aabbccddeeff00112233445566778899aabbccdd",
				Seeders:  "42", Leechers: "3", Size: "8589934592", Category: "207",
			},
		}
	}
	return &fixtureAPI{hits: hits}
}

func (f *fixtureAPI) Search(ctx context.Context, query string, cat int) ([]apibayHit, error) {
	_ = ctx
	_ = cat
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil, nil
	}
	var out []apibayHit
	for _, h := range f.hits {
		name := strings.ToLower(h.Name)
		if strings.Contains(name, strings.ReplaceAll(q, " ", ".")) ||
			strings.Contains(name, q) ||
			strings.Contains(strings.ReplaceAll(name, ".", " "), q) {
			out = append(out, h)
		}
	}
	if len(out) == 0 {
		// Still return fixtures so automation smoke has something to score when query is broad.
		return append([]apibayHit(nil), f.hits...), nil
	}
	return out, nil
}
