package internal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// searchAPI is the upstream search surface (Apibay-compatible JSON).
type searchAPI interface {
	Search(ctx context.Context, query string, cat int) ([]apibayHit, error)
}

// detailAPI is optional; Apibay clients implement Detail via /t.php.
type detailAPI interface {
	Detail(ctx context.Context, id string) (*apibayHit, error)
}

// apibayHit mirrors the known Apibay /q.php and /t.php JSON object fields.
// Numeric fields are often strings in live responses; flexString tolerates both.
type apibayHit struct {
	ID       flexString `json:"id"`
	Name     string     `json:"name"`
	InfoHash string     `json:"info_hash"`
	Leechers flexString `json:"leechers"`
	Seeders  flexString `json:"seeders"`
	NumFiles flexString `json:"num_files"`
	Size     flexString `json:"size"`
	Username string     `json:"username"`
	Added    flexString `json:"added"`
	Status   string     `json:"status"`
	Category flexString `json:"category"`
	IMDB     string     `json:"imdb"`
	Descr    string     `json:"descr"`
}

// flexString unmarshals JSON string or number into a string.
type flexString string

func (f *flexString) UnmarshalJSON(b []byte) error {
	b = bytesTrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		*f = ""
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = flexString(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*f = flexString(n.String())
	return nil
}

func (f flexString) String() string { return string(f) }

func (f flexString) Int() int {
	n, _ := strconv.Atoi(strings.TrimSpace(string(f)))
	return n
}

func (f flexString) Int64() int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(string(f)), 10, 64)
	return n
}

func bytesTrimSpace(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}

const (
	defaultApibayMinInterval = 2 * time.Second
	defaultApibayCacheTTL    = 10 * time.Minute
	minApibayCooldown        = 15 * time.Minute
)

// rateLimitedError is returned when Apibay 429s; Search maps it to ResourceExhausted.
type rateLimitedError struct {
	until time.Time
	msg   string
}

func (e *rateLimitedError) Error() string {
	if e == nil {
		return "apibay rate limited"
	}
	until := e.until.UTC().Format(time.RFC3339)
	if e.msg == "" {
		return "apibay rate limited until " + until
	}
	return "apibay rate limited until " + until + ": " + e.msg
}

func isRateLimited(err error) bool {
	var e *rateLimitedError
	return errors.As(err, &e)
}

type apibayCacheEntry struct {
	until time.Time
	hits  []apibayHit
}

// apibayClient talks to an Apibay-compatible base URL.
// If baseURL is empty, Search returns nil without making any HTTP request.
type apibayClient struct {
	lastCall      time.Time
	cooldownUntil time.Time
	http          *http.Client
	cache         map[string]apibayCacheEntry
	baseURL       string
	userAgent     string
	minInterval   time.Duration
	maxRetries    int
	cacheTTL      time.Duration
	mu            sync.Mutex
}

func envDuration(key string, fallback time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}

func newApibayClient(baseURL string, hc *http.Client, version string) *apibayClient {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if hc == nil && baseURL == "" {
		hc = &http.Client{Timeout: 20 * time.Second}
	}
	if version == "" {
		version = Version
	}
	return &apibayClient{
		baseURL:     baseURL,
		http:        hc,
		userAgent:   "muxcore-indexer-piratebay/" + version,
		minInterval: envDuration("PIRATEBAY_MIN_INTERVAL", defaultApibayMinInterval),
		maxRetries:  3,
		cacheTTL:    envDuration("PIRATEBAY_CACHE_TTL", defaultApibayCacheTTL),
		cache:       make(map[string]apibayCacheEntry),
	}
}

func (c *apibayClient) configured() bool {
	return c != nil && c.baseURL != ""
}

func (c *apibayClient) waitRate(ctx context.Context) error {
	c.mu.Lock()
	wait := c.minInterval - time.Since(c.lastCall)
	c.mu.Unlock()
	if wait <= 0 {
		return nil
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (c *apibayClient) markCall() {
	c.mu.Lock()
	c.lastCall = time.Now()
	c.mu.Unlock()
}

func (c *apibayClient) cacheKey(query string, cat int) string {
	return strconv.Itoa(cat) + "\x00" + strings.ToLower(strings.TrimSpace(query))
}

func (c *apibayClient) cacheGet(query string, cat int) ([]apibayHit, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cacheTTL <= 0 || c.cache == nil {
		return nil, false
	}
	ent, ok := c.cache[c.cacheKey(query, cat)]
	if !ok || time.Now().After(ent.until) {
		return nil, false
	}
	out := make([]apibayHit, len(ent.hits))
	copy(out, ent.hits)
	return out, true
}

func (c *apibayClient) cachePut(query string, cat int, hits []apibayHit) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cacheTTL <= 0 {
		return
	}
	if c.cache == nil {
		c.cache = make(map[string]apibayCacheEntry)
	}
	stored := make([]apibayHit, len(hits))
	copy(stored, hits)
	c.cache[c.cacheKey(query, cat)] = apibayCacheEntry{hits: stored, until: time.Now().Add(c.cacheTTL)}
}

func (c *apibayClient) Search(ctx context.Context, query string, cat int) ([]apibayHit, error) {
	if !c.configured() {
		return nil, nil
	}
	if c.http == nil {
		return nil, fmt.Errorf("apibay search: no HTTP client (VPN bind required)")
	}
	if hits, ok := c.cacheGet(query, cat); ok {
		return hits, nil
	}
	u, err := url.Parse(c.baseURL + "/q.php")
	if err != nil {
		return nil, fmt.Errorf("parse base URL: %w", err)
	}
	q := u.Query()
	q.Set("q", query)
	q.Set("cat", strconv.Itoa(cat))
	u.RawQuery = q.Encode()

	body, err := c.getJSON(ctx, u.String(), "apibay search")
	if err != nil {
		return nil, err
	}
	hits, err := parseApibayHits(body)
	if err != nil {
		return nil, err
	}
	c.cachePut(query, cat, hits)
	return hits, nil
}

// Detail fetches a single torrent via Apibay /t.php?id=.
func (c *apibayClient) Detail(ctx context.Context, id string) (*apibayHit, error) {
	if !c.configured() {
		return nil, nil
	}
	id = strings.TrimSpace(id)
	if id == "" || id == "0" {
		return nil, nil
	}
	u, err := url.Parse(c.baseURL + "/t.php")
	if err != nil {
		return nil, fmt.Errorf("parse base URL: %w", err)
	}
	q := u.Query()
	q.Set("id", id)
	u.RawQuery = q.Encode()

	body, err := c.getJSON(ctx, u.String(), "apibay detail")
	if err != nil {
		return nil, err
	}
	return parseApibayDetail(body)
}

func parseRetryAfter(h http.Header, now time.Time) time.Duration {
	raw := strings.TrimSpace(h.Get("Retry-After"))
	if raw == "" {
		return minApibayCooldown
	}
	if secs, err := strconv.Atoi(raw); err == nil {
		d := time.Duration(secs) * time.Second
		if d < minApibayCooldown {
			return minApibayCooldown
		}
		return d
	}
	if t, err := http.ParseTime(raw); err == nil {
		d := t.Sub(now)
		if d < minApibayCooldown {
			return minApibayCooldown
		}
		return d
	}
	return minApibayCooldown
}

func (c *apibayClient) setCooldown(d time.Duration, msg string) *rateLimitedError {
	if d < minApibayCooldown {
		d = minApibayCooldown
	}
	until := time.Now().Add(d)
	c.mu.Lock()
	c.cooldownUntil = until
	c.mu.Unlock()
	return &rateLimitedError{until: until, msg: msg}
}

func (c *apibayClient) getJSON(ctx context.Context, rawURL, op string) ([]byte, error) {
	c.mu.Lock()
	until := c.cooldownUntil
	c.mu.Unlock()
	if !until.IsZero() && time.Now().Before(until) {
		return nil, &rateLimitedError{until: until, msg: op}
	}

	var lastErr error
	backoff := 500 * time.Millisecond
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if err := c.waitRate(ctx); err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, http.NoBody)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		if c.userAgent != "" {
			req.Header.Set("User-Agent", c.userAgent)
		}

		c.markCall()
		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("%s: %w", op, err)
			if attempt == c.maxRetries {
				break
			}
			if err := sleepBackoff(ctx, backoff); err != nil {
				return nil, err
			}
			backoff *= 2
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		if closeErr := resp.Body.Close(); closeErr != nil && readErr == nil {
			readErr = closeErr
		}
		if readErr != nil {
			return nil, fmt.Errorf("read body: %w", readErr)
		}
		switch {
		case resp.StatusCode == http.StatusOK:
			return body, nil
		case resp.StatusCode == http.StatusTooManyRequests:
			msg := truncate(string(body), 200)
			return nil, c.setCooldown(parseRetryAfter(resp.Header, time.Now()), fmt.Sprintf("%s status 429: %s", op, msg))
		case resp.StatusCode >= 500:
			lastErr = fmt.Errorf("%s status %d: %s", op, resp.StatusCode, truncate(string(body), 200))
			if attempt == c.maxRetries {
				break
			}
			if err := sleepBackoff(ctx, backoff); err != nil {
				return nil, err
			}
			backoff *= 2
			continue
		default:
			return nil, fmt.Errorf("%s status %d: %s", op, resp.StatusCode, truncate(string(body), 200))
		}
	}
	return nil, lastErr
}

func sleepBackoff(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func parseApibayHits(body []byte) ([]apibayHit, error) {
	body = bytesTrimSpace(body)
	if len(body) == 0 {
		return nil, nil
	}
	var hits []apibayHit
	if err := json.Unmarshal(body, &hits); err != nil {
		return nil, fmt.Errorf("decode apibay json: %w", err)
	}
	// Apibay returns a single placeholder object with id "0" when empty.
	out := make([]apibayHit, 0, len(hits))
	for _, h := range hits {
		if h.ID.String() == "0" || h.InfoHash == "" || h.InfoHash == strings.Repeat("0", 40) {
			continue
		}
		out = append(out, h)
	}
	return out, nil
}

func parseApibayDetail(body []byte) (*apibayHit, error) {
	body = bytesTrimSpace(body)
	if len(body) == 0 {
		return nil, nil
	}
	var h apibayHit
	if err := json.Unmarshal(body, &h); err != nil {
		return nil, fmt.Errorf("decode apibay detail: %w", err)
	}
	if h.ID.String() == "0" || h.InfoHash == "" || h.InfoHash == strings.Repeat("0", 40) {
		return nil, nil
	}
	return &h, nil
}

// mergeDetail overlays /t.php fields onto a search hit (fresher seeders/size/imdb + descr).
func mergeDetail(base apibayHit, detail *apibayHit) apibayHit {
	if detail == nil {
		return base
	}
	out := base
	if detail.Name != "" {
		out.Name = detail.Name
	}
	if detail.InfoHash != "" {
		out.InfoHash = detail.InfoHash
	}
	if detail.Seeders.String() != "" {
		out.Seeders = detail.Seeders
	}
	if detail.Leechers.String() != "" {
		out.Leechers = detail.Leechers
	}
	if detail.Size.String() != "" {
		out.Size = detail.Size
	}
	if detail.NumFiles.String() != "" {
		out.NumFiles = detail.NumFiles
	}
	if detail.Added.String() != "" {
		out.Added = detail.Added
	}
	if detail.Category.String() != "" {
		out.Category = detail.Category
	}
	if detail.IMDB != "" {
		out.IMDB = detail.IMDB
	}
	if detail.Username != "" {
		out.Username = detail.Username
	}
	if detail.Descr != "" {
		out.Descr = detail.Descr
	}
	if detail.Status != "" {
		out.Status = detail.Status
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
