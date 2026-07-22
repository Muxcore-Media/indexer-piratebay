package internal

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	indexerv1 "github.com/Muxcore-Media/contracts-indexer/muxcore/indexer/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	indexerName = "The Pirate Bay"
	indexerID   = int32(1)
)

// categoryForSearch maps IndexerService search type / categories to Apibay cat codes.
// 0 = all. Common: 200=Video, 201=Movies, 205=TV, 100=Audio.
func categoryForSearch(searchType string, categories []string) int {
	for _, c := range categories {
		c = strings.ToLower(strings.TrimSpace(c))
		switch c {
		case "movie", "movies", "201":
			return 201
		case "tv", "television", "205":
			return 205
		case "audio", "music", "100":
			return 100
		case "video", "200":
			return 200
		}
	}
	switch strings.ToLower(strings.TrimSpace(searchType)) {
	case "movie":
		return 201
	case "tv":
		return 205
	case "audio", "music":
		return 100
	default:
		return 0
	}
}

func categoryLabel(cat flexString) (category, sub string) {
	n := cat.Int()
	switch {
	case n >= 200 && n < 300:
		category = "video"
		switch n {
		case 201:
			sub = "movies"
		case 205:
			sub = "tv"
		default:
			sub = "other"
		}
	case n >= 100 && n < 200:
		category = "audio"
		sub = "music"
	default:
		category = strconv.Itoa(n)
	}
	return category, sub
}

func hitToSearchResult(h apibayHit, apiBase string) *indexerv1.SearchResult {
	name := strings.TrimSpace(h.Name)
	hash := strings.TrimSpace(h.InfoHash)
	magnet := fmt.Sprintf("magnet:?xt=urn:btih:%s&dn=%s", hash, url.QueryEscape(name))

	infoURL := ""
	if apiBase != "" && h.ID.String() != "" {
		infoURL = strings.TrimRight(apiBase, "/") + "/t/" + url.PathEscape(h.ID.String())
	}

	cat, sub := categoryLabel(h.Category)
	var pub *timestamppb.Timestamp
	if added := h.Added.Int64(); added > 0 {
		pub = timestamppb.New(time.Unix(added, 0).UTC())
	}

	imdb := strings.TrimSpace(h.IMDB)
	if imdb != "" && !strings.HasPrefix(imdb, "tt") {
		imdb = "tt" + imdb
	}

	return &indexerv1.SearchResult{
		Guid:             h.ID.String(),
		Title:            name,
		InfoUrl:          infoURL,
		DownloadUrl:      magnet,
		Size:             h.Size.Int64(),
		Seeders:          int32(h.Seeders.Int()),
		Peers:            int32(h.Leechers.Int()),
		IndexerName:      indexerName,
		IndexerId:        indexerID,
		PublishDate:      pub,
		Category:         cat,
		SubCategory:      sub,
		ImdbId:           imdb,
		DownloadProtocol: "torrent",
	}
}

func mapHits(hits []apibayHit, apiBase string, offset, limit int) []*indexerv1.SearchResult {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	if offset >= len(hits) {
		return nil
	}
	hits = hits[offset:]
	if len(hits) > limit {
		hits = hits[:limit]
	}
	out := make([]*indexerv1.SearchResult, 0, len(hits))
	for _, h := range hits {
		out = append(out, hitToSearchResult(h, apiBase))
	}
	return out
}
