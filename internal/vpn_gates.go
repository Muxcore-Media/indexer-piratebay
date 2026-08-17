package internal

import (
	"fmt"
	"os"
	"strings"
)

func envFlagTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func indexerFixtureMode(cfgFlag bool) bool {
	if cfgFlag {
		return true
	}
	return envFlagTruthy(os.Getenv("INDEXER_FIXTURE"))
}

func validateWGConfReadable(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("WG_CONF is required for live Apibay HTTP (set WG_CONF, or INDEXER_FIXTURE=1 / leave PIRATEBAY_API_BASE empty)")
	}
	fi, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("WG_CONF missing or unreadable (%s): %w", path, err)
	}
	if fi.IsDir() {
		return fmt.Errorf("WG_CONF is a directory, not a file: %s", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("WG_CONF unreadable (%s): %w", path, err)
	}
	_ = f.Close()
	return nil
}

// liveHTTPRequiresVPN is true when a remote Apibay base is configured outside fixture/test inject paths.
func liveHTTPRequiresVPN(baseURL string, fixture bool, httpInjected bool) bool {
	if fixture || httpInjected {
		return false
	}
	return strings.TrimSpace(baseURL) != ""
}

func enforceLiveIndexerVPN(wgConf, baseURL string, fixture, httpInjected bool) error {
	if !liveHTTPRequiresVPN(baseURL, fixture, httpInjected) {
		return nil
	}
	path := strings.TrimSpace(wgConf)
	if path == "" {
		path = strings.TrimSpace(os.Getenv("WG_CONF"))
	}
	return validateWGConfReadable(path)
}
