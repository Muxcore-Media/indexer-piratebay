package internal

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"

	indexerv1 "github.com/Muxcore-Media/contracts-indexer/muxcore/indexer/v1"
	"github.com/Muxcore-Media/core/pkg/contracts"
)

type Module struct {
	indexerv1.UnimplementedIndexerServiceServer

	mu           sync.RWMutex
	api          searchAPI
	baseURL      string
	id           string
	grpcAddr     string
	grpcSrv      *grpc.Server
	lis          net.Listener
	http         *http.Client
	enrich       bool
	enrichLimit  int
	fixtureMode  bool
	httpInjected bool
	wgConfPath   string
	vpnIface     string
}

type Config struct {
	ID          string
	GRPCAddr    string
	BaseURL     string
	Timeout     time.Duration
	HTTP        *http.Client // optional; for tests inject httptest transport
	API         searchAPI    // optional; overrides Apibay client when set
	Enrich      bool         // optional; when set, overrides env default
	EnrichSet   bool         // true if Enrich was explicitly provided
	EnrichLimit int          // max hits to enrich (0 → default 5)
	Fixture     bool         // INDEXER_FIXTURE offline sample search
	WGConfPath  string       // WireGuard conf for live HTTP source-bind
	SkipVPNGate bool         // tests only: allow live base without WG
}

func NewModule(cfg Config) *Module {
	if cfg.ID == "" {
		cfg.ID = "indexer-piratebay"
	}
	if cfg.GRPCAddr == "" {
		cfg.GRPCAddr = ":9485"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 20 * time.Second
	}
	if v := os.Getenv("PIRATEBAY_GRPC_ADDR"); v != "" {
		cfg.GRPCAddr = v
	}
	fixture := indexerFixtureMode(cfg.Fixture)
	// Live upstream only when explicitly configured — empty means no network.
	// Config.BaseURL wins when set; otherwise fall back to PIRATEBAY_API_BASE.
	// Fixture mode clears live base unless API/HTTP was injected for unit tests.
	if cfg.BaseURL == "" && !fixture {
		if v := os.Getenv("PIRATEBAY_API_BASE"); v != "" {
			cfg.BaseURL = strings.TrimSpace(v)
		}
	}
	if fixture && cfg.API == nil && cfg.HTTP == nil {
		cfg.BaseURL = ""
	}
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")

	if cfg.WGConfPath == "" {
		cfg.WGConfPath = strings.TrimSpace(os.Getenv("WG_CONF"))
	}

	enrich := false
	if cfg.EnrichSet {
		enrich = cfg.Enrich
	} else if v := os.Getenv("PIRATEBAY_ENRICH_DETAILS"); v != "" {
		enrich = envTruthy(v)
	}
	enrichLimit := cfg.EnrichLimit
	if enrichLimit <= 0 {
		enrichLimit = 5
		if v := os.Getenv("PIRATEBAY_ENRICH_LIMIT"); v != "" {
			if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
				enrichLimit = n
			}
		}
	}
	if enrichLimit > 25 {
		enrichLimit = 25
	}

	httpInjected := cfg.HTTP != nil
	hc := cfg.HTTP
	vpnIface := ""
	if hc == nil && !fixture && cfg.BaseURL != "" && !cfg.SkipVPNGate {
		bound, iface, err := newVPNBoundHTTPClient(cfg.WGConfPath, cfg.Timeout)
		if err == nil {
			hc = bound
			vpnIface = iface
		}
		// If bind fails here (iface down), Init will surface the gate / bind error.
		if hc == nil {
			hc = &http.Client{Timeout: cfg.Timeout}
		}
	}
	if hc == nil {
		hc = &http.Client{Timeout: cfg.Timeout}
	}

	m := &Module{
		id:           cfg.ID,
		grpcAddr:     cfg.GRPCAddr,
		baseURL:      cfg.BaseURL,
		http:         hc,
		api:          cfg.API,
		enrich:       enrich,
		enrichLimit:  enrichLimit,
		fixtureMode:  fixture,
		httpInjected: httpInjected || cfg.SkipVPNGate,
		wgConfPath:   cfg.WGConfPath,
		vpnIface:     vpnIface,
	}
	if m.api == nil {
		if m.fixtureMode {
			m.api = newFixtureAPI()
		} else {
			m.api = newApibayClient(m.baseURL, hc)
		}
	}
	return m
}

func envTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func (m *Module) Info() contracts.ModuleInfo {
	return contracts.ModuleInfo{
		ID:           m.id,
		Name:         "Pirate Bay Indexer",
		Version:      "0.1.0",
		Roles:        []string{"indexer"},
		Description:  "Single-site The Pirate Bay indexer via Apibay-compatible JSON API",
		Author:       "MuxCore",
		Capabilities: []string{"indexer", "indexer.piratebay", "settings"},
		Contracts: []contracts.ContractDeclaration{
			{Repo: "github.com/Muxcore-Media/contracts-indexer", Interface: "Indexer", Version: "v1"},
		},
		MinCoreVersion: "0.4.0",
		HTTPAddr:       m.grpcAddr,
	}
}

func (m *Module) Init(ctx context.Context) error {
	if err := enforceLiveIndexerVPN(m.wgConfPath, m.baseURL, m.fixtureMode, m.httpInjected); err != nil {
		return err
	}
	// Live Apibay: rebuild HTTP client on the WG iface when not injected (shared policy with downloader).
	if liveHTTPRequiresVPN(m.baseURL, m.fixtureMode, m.httpInjected) && m.vpnIface == "" {
		bound, iface, err := newVPNBoundHTTPClient(m.wgConfPath, 20*time.Second)
		if err != nil {
			return fmt.Errorf("vpn-bound HTTP client: %w", err)
		}
		m.http = bound
		m.vpnIface = iface
		m.api = newApibayClient(m.baseURL, bound)
	}
	lis, err := net.Listen("tcp", m.grpcAddr)
	if err != nil {
		return fmt.Errorf("listen gRPC %s: %w", m.grpcAddr, err)
	}
	m.lis = lis
	// Prefer the actual bound address when using :0 in tests.
	if ta, ok := lis.Addr().(*net.TCPAddr); ok && strings.HasPrefix(m.grpcAddr, ":") {
		m.grpcAddr = fmt.Sprintf(":%d", ta.Port)
	}
	slog.Info("indexer-piratebay initialized",
		"grpc", m.grpcAddr,
		"configured", m.configured(),
		"fixture", m.fixtureMode,
		"vpn_iface", m.vpnIface,
		"enrich_details", m.enrich,
		"enrich_limit", m.enrichLimit,
	)
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	m.grpcSrv = grpc.NewServer()
	indexerv1.RegisterIndexerServiceServer(m.grpcSrv, m)
	m.registerSettingsMesh(m.grpcSrv)
	go func() {
		slog.Info("indexer-piratebay gRPC started", "addr", m.grpcAddr)
		if err := m.grpcSrv.Serve(m.lis); err != nil {
			slog.Error("indexer-piratebay gRPC serve error", "error", err)
		}
	}()
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	if m.grpcSrv != nil {
		m.grpcSrv.GracefulStop()
	}
	slog.Info("indexer-piratebay stopped")
	return nil
}

func (m *Module) Health(ctx context.Context) error {
	return nil
}

func (m *Module) configured() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.fixtureMode || m.baseURL != ""
}

func (m *Module) Search(ctx context.Context, req *indexerv1.SearchRequest) (*indexerv1.SearchResponse, error) {
	if !m.configured() {
		slog.Debug("indexer-piratebay: not configured (PIRATEBAY_API_BASE empty), returning no results")
		return &indexerv1.SearchResponse{}, nil
	}
	m.mu.RLock()
	base := m.baseURL
	fixture := m.fixtureMode
	injected := m.httpInjected
	wg := m.wgConfPath
	m.mu.RUnlock()
	if err := enforceLiveIndexerVPN(wg, base, fixture, injected); err != nil {
		return nil, err
	}
	query := strings.TrimSpace(req.GetQuery())
	if query == "" {
		return &indexerv1.SearchResponse{}, nil
	}
	if y := req.GetYear(); y > 0 && req.GetType() == "movie" {
		ys := strconv.Itoa(int(y))
		if !strings.Contains(query, ys) {
			query = query + " " + ys
		}
	}
	cat := categoryForSearch(req.GetType(), req.GetCategories())

	m.mu.RLock()
	api := m.api
	base = m.baseURL
	m.mu.RUnlock()

	hits, err := api.Search(ctx, query, cat)
	if err != nil {
		slog.Warn("indexer-piratebay search failed", "error", err)
		return nil, fmt.Errorf("search: %w", err)
	}
	page := pageHits(hits, int(req.GetOffset()), int(req.GetLimit()))
	m.mu.RLock()
	enrich := m.enrich
	enrichLimit := m.enrichLimit
	m.mu.RUnlock()
	if enrich {
		page = m.enrichHits(ctx, api, page, enrichLimit)
	}
	results := mapHits(page, base)
	return &indexerv1.SearchResponse{
		Results: results,
		Total:   int32(len(results)),
	}, nil
}

func (m *Module) enrichHits(ctx context.Context, api searchAPI, hits []apibayHit, limit int) []apibayHit {
	detail, ok := api.(detailAPI)
	if !ok || limit <= 0 || len(hits) == 0 {
		return hits
	}
	n := limit
	if n > len(hits) {
		n = len(hits)
	}
	out := make([]apibayHit, len(hits))
	copy(out, hits)
	for i := 0; i < n; i++ {
		id := hits[i].ID.String()
		d, err := detail.Detail(ctx, id)
		if err != nil {
			slog.Debug("indexer-piratebay detail enrich skipped", "id", id, "error", err)
			continue
		}
		if d == nil {
			continue
		}
		out[i] = mergeDetail(hits[i], d)
	}
	return out
}

func (m *Module) GetCapabilities(ctx context.Context, _ *indexerv1.GetCapabilitiesRequest) (*indexerv1.GetCapabilitiesResponse, error) {
	return &indexerv1.GetCapabilitiesResponse{
		SupportsSearch:      true,
		SupportsMovieSearch: true,
		SupportsTvSearch:    true,
		SupportedCategories: []string{
			"movie", "hd-movie", "tv", "hd-tv", "audio", "flac", "audiobook",
			"video", "ebook", "comics", "application", "game", "xxx", "other",
		},
		SupportedProtocols: []string{"torrent"},
	}, nil
}

func (m *Module) ListIndexers(ctx context.Context, _ *indexerv1.ListIndexersRequest) (*indexerv1.ListIndexersResponse, error) {
	return &indexerv1.ListIndexersResponse{
		Indexers: []*indexerv1.IndexerInfo{
			{
				Id:         indexerID,
				Name:       indexerName,
				Protocol:   "torrent",
				Language:   "en",
				Configured: m.configured(),
			},
		},
	}, nil
}
