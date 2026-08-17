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

func TestLiveInitRequiresWGConf(t *testing.T) {
	t.Setenv("WG_CONF", "")
	t.Setenv("INDEXER_FIXTURE", "")
	t.Setenv("PIRATEBAY_API_BASE", "")
	m := NewModule(Config{
		GRPCAddr: ":0",
		BaseURL:  "https://apibay.example.test",
	})
	err := m.Init(context.Background())
	if err == nil || !strings.Contains(err.Error(), "WG_CONF") {
		t.Fatalf("expected WG_CONF gate, got %v", err)
	}
}

func TestLiveInitRejectsMissingWGFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.conf")
	t.Setenv("INDEXER_FIXTURE", "")
	m := NewModule(Config{
		GRPCAddr:   ":0",
		BaseURL:    "https://apibay.example.test",
		WGConfPath: missing,
	})
	err := m.Init(context.Background())
	if err == nil || !strings.Contains(err.Error(), "WG_CONF") {
		t.Fatalf("expected missing WG_CONF error, got %v", err)
	}
}

func TestHTTPtestInjectSkipsVPNGate(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("testdata", "search_sample.json"))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	t.Setenv("WG_CONF", "")
	m := NewModule(Config{
		GRPCAddr: ":0",
		BaseURL:  srv.URL,
		HTTP:     srv.Client(),
	})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatalf("httptest inject should skip VPN: %v", err)
	}
	t.Cleanup(func() { _ = m.Stop(ctx) })
	resp, err := m.Search(ctx, &indexerv1.SearchRequest{Query: "Fight Club", Type: "movie", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Results) != 2 {
		t.Fatalf("results=%d", len(resp.Results))
	}
}

func TestFixtureModeOfflineSearchAndCaps(t *testing.T) {
	t.Setenv("WG_CONF", "")
	t.Setenv("PIRATEBAY_API_BASE", "https://should-not-be-used.example")
	m := NewModule(Config{GRPCAddr: ":0", Fixture: true})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop(ctx) })

	caps, err := m.GetCapabilities(ctx, &indexerv1.GetCapabilitiesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !caps.SupportsSearch || !caps.SupportsMovieSearch || !caps.SupportsTvSearch {
		t.Fatalf("caps: %+v", caps)
	}
	if len(caps.SupportedProtocols) == 0 || caps.SupportedProtocols[0] != "torrent" {
		t.Fatalf("protocols: %v", caps.SupportedProtocols)
	}

	list, err := m.ListIndexers(ctx, &indexerv1.ListIndexersRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !list.Indexers[0].Configured {
		t.Fatal("fixture should report configured=true")
	}

	resp, err := m.Search(ctx, &indexerv1.SearchRequest{Query: "Fight Club", Type: "movie", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Results) == 0 {
		t.Fatal("fixture search returned no results")
	}
	if !strings.Contains(resp.Results[0].DownloadUrl, "magnet:?xt=urn:btih:") {
		t.Errorf("magnet: %s", resp.Results[0].DownloadUrl)
	}
}

func TestDiscoveryWithoutLiveApibay(t *testing.T) {
	t.Setenv("WG_CONF", "")
	t.Setenv("PIRATEBAY_API_BASE", "")
	t.Setenv("INDEXER_FIXTURE", "")
	m := NewModule(Config{GRPCAddr: ":0", BaseURL: ""})
	info := m.Info()
	hasIndexer := false
	for _, c := range info.Capabilities {
		if c == "indexer" || c == "indexer.piratebay" {
			hasIndexer = true
			break
		}
	}
	if !hasIndexer {
		t.Fatalf("capabilities must advertise indexer without live Apibay: %v", info.Capabilities)
	}
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop(ctx) })
	caps, err := m.GetCapabilities(ctx, &indexerv1.GetCapabilitiesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !caps.SupportsSearch {
		t.Fatal("GetCapabilities must work offline")
	}
	list, err := m.ListIndexers(ctx, &indexerv1.ListIndexersRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if list.Indexers[0].Configured {
		t.Fatal("unconfigured live should report configured=false")
	}
}

func TestIfaceFromWGConf(t *testing.T) {
	if got := ifaceFromWGConf("/etc/wireguard/wg-mux.conf"); got != "wg-mux" {
		t.Fatalf("got %q", got)
	}
}
