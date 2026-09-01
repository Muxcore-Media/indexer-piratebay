package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"time"

	indexerv1 "github.com/Muxcore-Media/contracts-indexer/muxcore/indexer/v1"
	"github.com/Muxcore-Media/core/sdk/go/client"
)

type storageWriter interface {
	Put(ctx context.Context, key string, r io.Reader) error
}

func (m *Module) meshStorage() storageWriter {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.storageOverride != nil {
		return m.storageOverride
	}
	if m.mc != nil && m.mc.Storage != nil {
		return m.mc.Storage
	}
	return nil
}

// torrentIndexEntry is stored under torrent/{infohash}/index.json so any
// StorageService backend (local, S3, …) can resolve search metadata by hash.
type torrentIndexEntry struct {
	InfoHash    string `json:"infohash"`
	Title       string `json:"title"`
	DownloadURL string `json:"download_url"`
	Indexer     string `json:"indexer,omitempty"`
	CachedAt    string `json:"cached_at"`
	Size        int64  `json:"size,omitempty"`
}

var btihRe = regexp.MustCompile(`(?i)(?:urn:btih:|btih[=:])([a-fA-F0-9]{40}|[a-zA-Z2-7]{32})`)

func parseInfoHashFromDownloadURL(u string) string {
	u = strings.TrimSpace(u)
	if u == "" {
		return ""
	}
	m := btihRe.FindStringSubmatch(u)
	if len(m) < 2 {
		return ""
	}
	return strings.ToLower(m[1])
}

func (m *Module) dialCore(ctx context.Context) {
	meshAddr := strings.TrimSpace(os.Getenv("MUXCORE_GRPC_ADDR"))
	if meshAddr == "" {
		return
	}
	insecureMode := os.Getenv("MUXCORE_INSECURE_DISABLE_TLS") == "true" || os.Getenv("MUXCORE_GRPC_INSECURE") == "true"
	var opts []client.Option
	if insecureMode {
		opts = append(opts, client.WithInsecure())
	}
	c, err := client.Dial(meshAddr, opts...)
	if err != nil {
		slog.Warn("indexer-piratebay: dial core for torrent index cache", "error", err)
		return
	}
	m.mu.Lock()
	if m.mc != nil {
		_ = m.mc.Close()
	}
	m.mc = c
	m.mu.Unlock()
	slog.Info("indexer-piratebay: mesh storage cache ready", "addr", meshAddr)
	_ = ctx
}

func (m *Module) cacheHitsInStorage(ctx context.Context, results []*indexerv1.SearchResult) {
	stor := m.meshStorage()
	if stor == nil || len(results) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for _, r := range results {
		if r == nil {
			continue
		}
		ih := parseInfoHashFromDownloadURL(r.GetDownloadUrl())
		if ih == "" || len(ih) != 40 {
			continue
		}
		ent := torrentIndexEntry{
			InfoHash:    ih,
			Title:       r.GetTitle(),
			DownloadURL: r.GetDownloadUrl(),
			Size:        r.GetSize(),
			Indexer:     r.GetIndexerName(),
			CachedAt:    time.Now().UTC().Format(time.RFC3339),
		}
		data, err := json.Marshal(ent)
		if err != nil {
			continue
		}
		key := fmt.Sprintf("torrent/%s/index.json", ih)
		if err := stor.Put(ctx, key, bytes.NewReader(data)); err != nil {
			slog.Debug("indexer-piratebay: cache index", "key", key, "error", err)
		}
	}
}
