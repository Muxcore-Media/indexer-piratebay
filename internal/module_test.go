package internal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	indexerv1 "github.com/Muxcore-Media/contracts-indexer/muxcore/indexer/v1"
)

func TestParseApibayHitsFixture(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("testdata", "search_sample.json"))
	if err != nil {
		t.Fatal(err)
	}
	hits, err := parseApibayHits(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 {
		t.Fatalf("expected 2 real hits (placeholder filtered), got %d", len(hits))
	}
	if hits[0].Name != "Fight.Club.1999.1080p.BluRay.x264" {
		t.Errorf("name: %q", hits[0].Name)
	}
	if hits[0].Seeders.Int() != 42 {
		t.Errorf("seeders string form: got %d", hits[0].Seeders.Int())
	}
	if hits[1].Seeders.Int() != 15 {
		t.Errorf("seeders number form: got %d", hits[1].Seeders.Int())
	}
	if hits[0].Size.Int64() != 8589934592 {
		t.Errorf("size: got %d", hits[0].Size.Int64())
	}
}

func TestApibayClientEmptyBaseNoNetwork(t *testing.T) {
	var dials int
	hc := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		dials++
		t.Fatal("HTTP must not be called when base URL is empty")
		return nil, nil
	})}
	c := newApibayClient("", hc)
	hits, err := c.Search(context.Background(), "Fight Club", 0)
	if err != nil {
		t.Fatal(err)
	}
	if hits != nil {
		t.Errorf("expected nil hits, got %v", hits)
	}
	if dials != 0 {
		t.Errorf("unexpected dials: %d", dials)
	}
}

func TestApibayClientHTTPtest(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("testdata", "search_sample.json"))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/q.php" {
			t.Errorf("path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("q") != "Fight Club" {
			t.Errorf("q: %s", r.URL.Query().Get("q"))
		}
		if r.URL.Query().Get("cat") != "201" {
			t.Errorf("cat: %s", r.URL.Query().Get("cat"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	c := newApibayClient(srv.URL, srv.Client())
	hits, err := c.Search(context.Background(), "Fight Club", 201)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 {
		t.Fatalf("got %d hits", len(hits))
	}
}

func TestHitToSearchResultMagnet(t *testing.T) {
	h := apibayHit{
		ID:       "99",
		Name:     "Foo Bar",
		InfoHash: "ABCDEF0123456789ABCDEF0123456789ABCDEF01",
		Seeders:  "10",
		Leechers: "2",
		Size:     "1000",
		Category: "201",
		IMDB:     "0137523",
		Added:    "1609459200",
	}
	r := hitToSearchResult(h, "http://example.test")
	if !strings.HasPrefix(r.DownloadUrl, "magnet:?xt=urn:btih:ABCDEF0123456789ABCDEF0123456789ABCDEF01") {
		t.Errorf("magnet: %s", r.DownloadUrl)
	}
	if r.IndexerName != indexerName || r.IndexerId != indexerID {
		t.Errorf("indexer identity: %+v", r)
	}
	if r.DownloadProtocol != "torrent" {
		t.Errorf("protocol: %s", r.DownloadProtocol)
	}
	if r.ImdbId != "tt0137523" {
		t.Errorf("imdb: %s", r.ImdbId)
	}
	if r.Category != "video" || r.SubCategory != "movies" {
		t.Errorf("category: %s/%s", r.Category, r.SubCategory)
	}
	if r.InfoUrl != "http://example.test/t/99" {
		t.Errorf("info_url: %s", r.InfoUrl)
	}
}

func TestCategoryForSearch(t *testing.T) {
	if got := categoryForSearch("movie", nil); got != 201 {
		t.Errorf("movie -> %d", got)
	}
	if got := categoryForSearch("tv", nil); got != 205 {
		t.Errorf("tv -> %d", got)
	}
	if got := categoryForSearch("search", nil); got != 0 {
		t.Errorf("search -> %d", got)
	}
	if got := categoryForSearch("", []string{"audio"}); got != 100 {
		t.Errorf("audio cat -> %d", got)
	}
}

func TestMapHitsLimitOffset(t *testing.T) {
	hits := []apibayHit{
		{ID: "1", Name: "A", InfoHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		{ID: "2", Name: "B", InfoHash: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
		{ID: "3", Name: "C", InfoHash: "cccccccccccccccccccccccccccccccccccccccc"},
	}
	out := mapHits(pageHits(hits, 1, 1), "")
	if len(out) != 1 || out[0].Guid != "2" {
		t.Fatalf("got %+v", out)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSearchUnconfigured(t *testing.T) {
	t.Setenv("PIRATEBAY_API_BASE", "")
	m := NewModule(Config{GRPCAddr: ":0", BaseURL: ""})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop(ctx) })

	resp, err := m.Search(ctx, &indexerv1.SearchRequest{Query: "Fight Club", Type: "movie"})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Results) != 0 {
		t.Errorf("expected empty, got %d", len(resp.Results))
	}
	list, err := m.ListIndexers(ctx, &indexerv1.ListIndexersRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if list.Indexers[0].Configured {
		t.Error("expected configured=false")
	}
}

func TestSearchViaHTTPtest(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("testdata", "search_sample.json"))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	m := NewModule(Config{
		GRPCAddr: ":0",
		BaseURL:  srv.URL,
		HTTP:     srv.Client(),
	})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop(ctx) })

	resp, err := m.Search(ctx, &indexerv1.SearchRequest{
		Query: "Fight Club",
		Type:  "movie",
		Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(resp.Results))
	}
	if !strings.Contains(resp.Results[0].DownloadUrl, "magnet:?xt=urn:btih:") {
		t.Errorf("download_url: %s", resp.Results[0].DownloadUrl)
	}

	var gotQuery string
	srvYear := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("q")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srvYear.Close)
	mYear := NewModule(Config{
		GRPCAddr: ":0",
		BaseURL:  srvYear.URL,
		HTTP:     srvYear.Client(),
	})
	if err := mYear.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := mYear.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = mYear.Stop(ctx) })
	if _, err := mYear.Search(ctx, &indexerv1.SearchRequest{
		Query: "Fight Club",
		Type:  "movie",
		Year:  1999,
		Limit: 10,
	}); err != nil {
		t.Fatal(err)
	}
	if gotQuery != "Fight Club 1999" {
		t.Errorf("movie year should be appended to query, got %q", gotQuery)
	}

	caps, err := m.GetCapabilities(ctx, &indexerv1.GetCapabilitiesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !caps.SupportsMovieSearch || !caps.SupportsTvSearch {
		t.Errorf("capabilities: %+v", caps)
	}

	list, err := m.ListIndexers(ctx, &indexerv1.ListIndexersRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !list.Indexers[0].Configured {
		t.Error("expected configured=true")
	}
}

func TestModuleInfo(t *testing.T) {
	m := NewModule(Config{})
	info := m.Info()
	if info.ID != "indexer-piratebay" {
		t.Errorf("id: %s", info.ID)
	}
	if len(info.Capabilities) == 0 || info.Capabilities[0] != "indexer" {
		t.Errorf("capabilities: %v", info.Capabilities)
	}
	if info.HTTPAddr != ":9485" {
		t.Errorf("HTTPAddr: %s", info.HTTPAddr)
	}
}

func TestParseApibayDetailFixture(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("testdata", "detail_sample.json"))
	if err != nil {
		t.Fatal(err)
	}
	h, err := parseApibayDetail(body)
	if err != nil {
		t.Fatal(err)
	}
	if h == nil || h.ID.String() != "12345" {
		t.Fatalf("hit: %+v", h)
	}
	if h.Seeders.Int() != 99 {
		t.Errorf("seeders: %d", h.Seeders.Int())
	}
	if !strings.Contains(h.Descr, "NFO") {
		t.Errorf("descr: %q", h.Descr)
	}
}

func TestMergeDetail(t *testing.T) {
	base := apibayHit{
		ID: "12345", Name: "A", InfoHash: "aa", Seeders: "10", IMDB: "",
	}
	detail := &apibayHit{
		ID: "12345", Name: "A", InfoHash: "aa", Seeders: "99", IMDB: "0137523", Descr: "nfo",
	}
	got := mergeDetail(base, detail)
	if got.Seeders.Int() != 99 || got.IMDB != "0137523" || got.Descr != "nfo" {
		t.Fatalf("merged: %+v", got)
	}
}

func TestSearchEnrichDetailsOffByDefault(t *testing.T) {
	searchBody, err := os.ReadFile(filepath.Join("testdata", "search_sample.json"))
	if err != nil {
		t.Fatal(err)
	}
	var detailHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "t.php") {
			detailHits++
		}
		_, _ = w.Write(searchBody)
	}))
	t.Cleanup(srv.Close)

	m := NewModule(Config{
		GRPCAddr:  ":0",
		BaseURL:   srv.URL,
		HTTP:      srv.Client(),
		EnrichSet: true,
		Enrich:    false,
	})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop(ctx) })

	resp, err := m.Search(ctx, &indexerv1.SearchRequest{Query: "Fight Club", Type: "movie", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Results) != 2 {
		t.Fatalf("results=%d", len(resp.Results))
	}
	if detailHits != 0 {
		t.Fatalf("detail calls=%d want 0 when enrich off", detailHits)
	}
	if resp.Results[0].Seeders != 42 {
		t.Errorf("seeders=%d want search value 42", resp.Results[0].Seeders)
	}
}

func TestSearchEnrichDetailsOn(t *testing.T) {
	searchBody, err := os.ReadFile(filepath.Join("testdata", "search_sample.json"))
	if err != nil {
		t.Fatal(err)
	}
	detailBody, err := os.ReadFile(filepath.Join("testdata", "detail_sample.json"))
	if err != nil {
		t.Fatal(err)
	}
	var detailHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/t.php"):
			detailHits++
			_, _ = w.Write(detailBody)
		default:
			_, _ = w.Write(searchBody)
		}
	}))
	t.Cleanup(srv.Close)

	m := NewModule(Config{
		GRPCAddr:    ":0",
		BaseURL:     srv.URL,
		HTTP:        srv.Client(),
		EnrichSet:   true,
		Enrich:      true,
		EnrichLimit: 1,
	})
	if c, ok := m.api.(*apibayClient); ok {
		c.minInterval = 0
		c.maxRetries = 0
	}
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop(ctx) })

	resp, err := m.Search(ctx, &indexerv1.SearchRequest{Query: "Fight Club", Type: "movie", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if detailHits != 1 {
		t.Fatalf("detail calls=%d want 1 (enrich_limit=1)", detailHits)
	}
	if resp.Results[0].Seeders != 99 {
		t.Errorf("enriched seeders=%d want 99", resp.Results[0].Seeders)
	}
	if !strings.Contains(resp.Results[0].InfoUrl, "t.php?id=") {
		t.Errorf("info_url=%q want t.php when descr present", resp.Results[0].InfoUrl)
	}
	if resp.Results[1].Seeders != 15 {
		t.Errorf("second hit should stay unenriched seeders=%d", resp.Results[1].Seeders)
	}
}
