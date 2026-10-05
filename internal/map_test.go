package internal

import (
	"context"
	"strings"
	"testing"

	indexerv1 "github.com/Muxcore-Media/contracts-indexer/muxcore/indexer/v1"
)

// fixtureGrabAllowed replicates fixtureIndexerGrab from
// media-automation/internal/dispatch_guard.go (internal package, not importable).
func fixtureGrabAllowed(indexerName, guid, downloadURL string) bool {
	for _, s := range []string{indexerName, guid, downloadURL} {
		if strings.Contains(strings.ToLower(strings.TrimSpace(s)), "fixture") {
			return true
		}
	}
	return false
}

func TestMarkFixtureResultsAcceptedByDispatchGuard(t *testing.T) {
	hits := []apibayHit{{ID: "99", Name: "Foo", InfoHash: "ABCDEF0123456789ABCDEF0123456789ABCDEF01"}}
	res := mapHits(hits, "")
	markFixtureResults(res)
	r := res[0]
	if r.IndexerName != "The Pirate Bay (fixture)" || r.Guid != "fixture-99" {
		t.Fatalf("marker: name=%q guid=%q", r.IndexerName, r.Guid)
	}
	if !fixtureGrabAllowed(r.IndexerName, r.Guid, r.DownloadUrl) {
		t.Fatal("dispatch guard predicate rejects fixture result")
	}
	if !strings.HasPrefix(r.DownloadUrl, "magnet:?xt=urn:btih:ABCDEF") {
		t.Errorf("magnet changed: %s", r.DownloadUrl)
	}
}

func TestFixtureModuleSearchMarksResults(t *testing.T) {
	m := NewModule(Config{GRPCAddr: ":0", Fixture: true})
	resp, err := m.Search(context.Background(), &indexerv1.SearchRequest{Query: "Fight Club", Type: "movie"})
	if err != nil || len(resp.Results) == 0 {
		t.Fatalf("fixture search: %v, %d results", err, len(resp.GetResults()))
	}
	for _, r := range resp.Results {
		if !fixtureGrabAllowed(r.IndexerName, r.Guid, r.DownloadUrl) {
			t.Errorf("unmarked fixture result: %+v", r)
		}
	}
}

func TestLiveMappingUnchanged(t *testing.T) {
	res := mapHits([]apibayHit{{ID: "99", Name: "Foo", InfoHash: "ABCDEF0123456789ABCDEF0123456789ABCDEF01"}}, "")
	r := res[0]
	if r.IndexerName != "The Pirate Bay" || r.Guid != "99" {
		t.Fatalf("live mapping changed: name=%q guid=%q", r.IndexerName, r.Guid)
	}
	if fixtureGrabAllowed(r.IndexerName, r.Guid, r.DownloadUrl) {
		t.Fatal("live result must not satisfy the fixture guard")
	}
}
