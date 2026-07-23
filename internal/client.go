package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// searchAPI is the upstream search surface (Apibay-compatible JSON).
type searchAPI interface {
	Search(ctx context.Context, query string, cat int) ([]apibayHit, error)
}

// apibayHit mirrors the known Apibay /q.php JSON object fields.
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

// apibayClient talks to an Apibay-compatible base URL.
// If baseURL is empty, Search returns nil without making any HTTP request.
type apibayClient struct {
	baseURL    string
	http       *http.Client
	mu         sync.Mutex
	lastCall   time.Time
	minInterval time.Duration
	maxRetries int
}

func newApibayClient(baseURL string, hc *http.Client) *apibayClient {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if hc == nil {
		hc = &http.Client{Timeout: 20 * time.Second}
	}
	return &apibayClient{
		baseURL:     baseURL,
		http:        hc,
		minInterval: 500 * time.Millisecond,
		maxRetries:  3,
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

func (c *apibayClient) Search(ctx context.Context, query string, cat int) ([]apibayHit, error) {
	if !c.configured() {
		return nil, nil
	}
	u, err := url.Parse(c.baseURL + "/q.php")
	if err != nil {
		return nil, fmt.Errorf("parse base URL: %w", err)
	}
	q := u.Query()
	q.Set("q", query)
	q.Set("cat", strconv.Itoa(cat))
	u.RawQuery = q.Encode()

	var lastErr error
	backoff := 500 * time.Millisecond
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if err := c.waitRate(ctx); err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")

		c.markCall()
		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("apibay search: %w", err)
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
		resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read body: %w", readErr)
		}
		switch {
		case resp.StatusCode == http.StatusOK:
			return parseApibayHits(body)
		case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
			lastErr = fmt.Errorf("apibay status %d: %s", resp.StatusCode, truncate(string(body), 200))
			if attempt == c.maxRetries {
				break
			}
			if err := sleepBackoff(ctx, backoff); err != nil {
				return nil, err
			}
			backoff *= 2
			continue
		default:
			return nil, fmt.Errorf("apibay status %d: %s", resp.StatusCode, truncate(string(body), 200))
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

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
