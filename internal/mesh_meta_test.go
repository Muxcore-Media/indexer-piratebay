package internal

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	indexerv1 "github.com/Muxcore-Media/contracts-indexer/muxcore/indexer/v1"
)

type fakeStoragePut struct {
	mu   sync.Mutex
	puts map[string][]byte
}

func (f *fakeStoragePut) Put(_ context.Context, key string, r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.puts == nil {
		f.puts = make(map[string][]byte)
	}
	f.puts[key] = data
	return nil
}

func (f *fakeStoragePut) get(key string) ([]byte, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, ok := f.puts[key]
	return data, ok
}

func TestCacheHitsInStorageSurvivesCancelledCtx(t *testing.T) {
	ih := "aabbccddeeff00112233445566778899aabbccdd"
	store := &fakeStoragePut{}
	m := NewModule(Config{GRPCAddr: ":0", Fixture: true})
	m.storageOverride = store

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	results := []*indexerv1.SearchResult{
		{
			Title:       "Fight Club",
			DownloadUrl: "magnet:?xt=urn:btih:" + ih,
			IndexerName: indexerName,
		},
	}
	m.cacheHitsInStorage(context.WithoutCancel(ctx), results)

	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, ok := store.get("torrent/" + ih + "/index.json"); ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("expected torrent index write via WithoutCancel ctx")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestSearchCacheHitsWithCancelledCtx(t *testing.T) {
	ih := "abcdef0123456789abcdef0123456789abcdef01"
	store := &fakeStoragePut{}
	m := NewModule(Config{GRPCAddr: ":0", Fixture: true})
	m.storageOverride = store

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	resp, err := m.Search(ctx, &indexerv1.SearchRequest{Query: "Fight Club", Type: "movie", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Results) == 0 {
		t.Fatal("expected fixture results")
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, ok := store.get("torrent/" + ih + "/index.json"); ok {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("Search should cache hits even when caller ctx is cancelled")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestParseInfoHashFromDownloadURL(t *testing.T) {
	ih := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	got := parseInfoHashFromDownloadURL("magnet:?xt=urn:btih:" + ih + "&dn=x")
	if got != ih {
		t.Fatalf("got %q", got)
	}
	if parseInfoHashFromDownloadURL("http://example/download") != "" {
		t.Fatal("expected empty for http")
	}
}

func TestModuleInfoUsesVersion(t *testing.T) {
	m := NewModule(Config{Version: "1.2.3"})
	info := m.Info()
	if info.Version != "1.2.3" {
		t.Fatalf("version=%q want 1.2.3", info.Version)
	}
	if info.Contracts[0].Version != "v0.1.0" {
		t.Fatalf("contract version=%q want v0.1.0", info.Contracts[0].Version)
	}
	if info.MinCoreVersion != "0.5.8" {
		t.Fatalf("min core=%q", info.MinCoreVersion)
	}
}

func TestApibayClientUserAgentDefaultVersion(t *testing.T) {
	var ua string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua = r.Header.Get("User-Agent")
		_, _ = w.Write([]byte("[]"))
	}))
	t.Cleanup(srv.Close)

	Version = "9.9.9"
	c := newApibayClient(srv.URL, srv.Client(), "")
	c.minInterval = 0
	if _, err := c.Search(context.Background(), "x", 0); err != nil {
		t.Fatal(err)
	}
	if ua != "muxcore-indexer-piratebay/9.9.9" {
		t.Fatalf("User-Agent=%q", ua)
	}
}

func TestAppendTVQueryTokens(t *testing.T) {
	got := appendTVQueryTokens("Show", 2, 0, 0)
	if !strings.Contains(got, "S02") {
		t.Fatalf("season pack: %q", got)
	}
	got = appendTVQueryTokens("Show", 1, 3, 0)
	if !strings.Contains(got, "S01E03") {
		t.Fatalf("episode: %q", got)
	}
	got = appendTVQueryTokens("Show S01E03", 1, 3, 0)
	if strings.Count(strings.ToUpper(got), "S01E03") > 1 {
		t.Fatalf("duplicate episode token: %q", got)
	}
	got = appendTVQueryTokens("Anime", 0, 0, 42)
	if !strings.Contains(got, "42") {
		t.Fatalf("absolute: %q", got)
	}
}
