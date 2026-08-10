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

// Apibay top-level category codes (partial tree used by search).
// 100 Audio, 101 Music, 102 Audio books, 103 Sound clips, 104 FLAC, 199 Other
// 200 Video, 201 Movies, 202 Movies DVDR, 203 Music videos, 204 Movie clips,
// 205 TV shows, 206 Handheld, 207 HD Movies, 208 HD TV, 209 3D, 299 Other
// 300 Applications, 301 Windows, 302 Mac, 303 UNIX, 304 Handheld, 305 iOS, 306 Android, 399 Other
// 400 Games, 401 PC, 402 Mac, 403 PSx, 404 XBOX360, 405 Wii, 406 Handheld, 407 iOS, 408 Android, 499 Other
// 500 Porn, 501 Movies, 502 Movies DVDR, 503 Pictures, 504 Games, 505 HD Movies, 506 Movie clips, 599 Other
// 600 Other, 601 E-books, 602 Comics, 603 Pictures, 604 Covers, 605 Physibles, 699 Other

// categoryForSearch maps IndexerService search type / categories to Apibay cat codes.
// 0 = all.
func categoryForSearch(searchType string, categories []string) int {
	for _, c := range categories {
		c = strings.ToLower(strings.TrimSpace(c))
		if n, err := strconv.Atoi(c); err == nil && n >= 0 {
			return n
		}
		switch c {
		case "movie", "movies":
			return 201
		case "hd-movie", "hd_movie", "hdmovies":
			return 207
		case "tv", "television", "tvshows", "tv-shows":
			return 205
		case "hd-tv", "hd_tv", "hdtv":
			return 208
		case "audio", "music":
			return 100
		case "flac":
			return 104
		case "audiobook", "audiobooks", "audio-book":
			return 102
		case "video":
			return 200
		case "ebook", "ebooks", "e-book", "books", "book":
			return 601
		case "comics", "comic":
			return 602
		case "application", "applications", "apps", "app":
			return 300
		case "game", "games", "pc-games":
			return 400
		case "xxx", "porn":
			return 500
		case "other":
			return 600
		}
	}
	switch strings.ToLower(strings.TrimSpace(searchType)) {
	case "movie":
		return 201
	case "tv":
		return 205
	case "audio", "music":
		return 100
	case "book", "ebook":
		return 601
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
		case 201, 202, 207, 209:
			sub = "movies"
		case 205, 208:
			sub = "tv"
		case 203:
			sub = "music-videos"
		default:
			sub = "other"
		}
	case n >= 100 && n < 200:
		category = "audio"
		switch n {
		case 102:
			sub = "audiobooks"
		case 104:
			sub = "flac"
		default:
			sub = "music"
		}
	case n >= 300 && n < 400:
		category = "applications"
		sub = "other"
	case n >= 400 && n < 500:
		category = "games"
		sub = "other"
	case n >= 500 && n < 600:
		category = "xxx"
		sub = "other"
	case n >= 600 && n < 700:
		category = "other"
		switch n {
		case 601:
			sub = "ebooks"
		case 602:
			sub = "comics"
		default:
			sub = "other"
		}
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
			// When detail enrich ran, point InfoUrl at /t.php (JSON includes descr/NFO).
		if strings.TrimSpace(h.Descr) != "" {
			infoURL = strings.TrimRight(apiBase, "/") + "/t.php?id=" + url.QueryEscape(h.ID.String())
		} else {
			infoURL = strings.TrimRight(apiBase, "/") + "/t/" + url.PathEscape(h.ID.String())
		}
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

func pageHits(hits []apibayHit, offset, limit int) []apibayHit {
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
	return hits
}

func mapHits(hits []apibayHit, apiBase string) []*indexerv1.SearchResult {
	out := make([]*indexerv1.SearchResult, 0, len(hits))
	for _, h := range hits {
		out = append(out, hitToSearchResult(h, apiBase))
	}
	return out
}
