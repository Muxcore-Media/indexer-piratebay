package internal

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"

	indexerv1 "github.com/Muxcore-Media/contracts-indexer/muxcore/indexer/v1"
	"github.com/Muxcore-Media/core/pkg/contracts"
)

type Module struct {
	indexerv1.UnimplementedIndexerServiceServer

	mu       sync.RWMutex
	api      searchAPI
	baseURL  string
	id       string
	grpcAddr string
	grpcSrv  *grpc.Server
	lis      net.Listener
	http     *http.Client
}

type Config struct {
	ID       string
	GRPCAddr string
	BaseURL  string
	Timeout  time.Duration
	HTTP     *http.Client // optional; for tests inject httptest transport
	API      searchAPI    // optional; overrides Apibay client when set
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
	// Live upstream only when explicitly configured — empty means no network.
	// Config.BaseURL wins when set; otherwise fall back to PIRATEBAY_API_BASE.
	if cfg.BaseURL == "" {
		if v := os.Getenv("PIRATEBAY_API_BASE"); v != "" {
			cfg.BaseURL = strings.TrimSpace(v)
		}
	}
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")

	hc := cfg.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: cfg.Timeout}
	}

	m := &Module{
		id:       cfg.ID,
		grpcAddr: cfg.GRPCAddr,
		baseURL:  cfg.BaseURL,
		http:     hc,
		api:      cfg.API,
	}
	if m.api == nil {
		m.api = newApibayClient(m.baseURL, hc)
	}
	return m
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
		"configured", m.baseURL != "",
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
	return m.baseURL != ""
}

func (m *Module) Search(ctx context.Context, req *indexerv1.SearchRequest) (*indexerv1.SearchResponse, error) {
	if !m.configured() {
		slog.Debug("indexer-piratebay: not configured (PIRATEBAY_API_BASE empty), returning no results")
		return &indexerv1.SearchResponse{}, nil
	}
	query := strings.TrimSpace(req.GetQuery())
	if query == "" {
		return &indexerv1.SearchResponse{}, nil
	}
	cat := categoryForSearch(req.GetType(), req.GetCategories())

	m.mu.RLock()
	api := m.api
	base := m.baseURL
	m.mu.RUnlock()

	hits, err := api.Search(ctx, query, cat)
	if err != nil {
		slog.Warn("indexer-piratebay search failed", "error", err)
		return nil, fmt.Errorf("search: %w", err)
	}
	results := mapHits(hits, base, int(req.GetOffset()), int(req.GetLimit()))
	return &indexerv1.SearchResponse{
		Results: results,
		Total:   int32(len(results)),
	}, nil
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
