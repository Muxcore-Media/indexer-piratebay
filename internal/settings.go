package internal

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Muxcore-Media/core/pkg/contracts"
	modulesdk "github.com/Muxcore-Media/core/sdk/go/module"
	"google.golang.org/grpc"
)

func (m *Module) settingsDefs() []contracts.SettingDef {
	m.mu.RLock()
	base := m.baseURL
	enrich := m.enrich
	limit := m.enrichLimit
	m.mu.RUnlock()
	enrichVal := "false"
	if enrich {
		enrichVal = "true"
	}
	return []contracts.SettingDef{
		{
			Key:         "base_url",
			Label:       "Apibay API Base URL",
			Type:        contracts.SettingTypeString,
			Default:     "",
			Value:       base,
			Description: "Base URL for the Apibay-compatible JSON API (e.g. https://apibay.org)",
			Required:    false,
			Group:       "Connection",
		},
		{
			Key:         "enrich_details",
			Label:       "Enrich search with torrent details",
			Type:        contracts.SettingTypeBool,
			Default:     "false",
			Value:       enrichVal,
			Description: "When true, fetch Apibay /t.php for the top hits (fresher seeders/size/imdb + NFO snippet). Off by default (extra upstream calls).",
			Required:    false,
			Group:       "Search",
		},
		{
			Key:         "enrich_limit",
			Label:       "Detail enrichment limit",
			Type:        contracts.SettingTypeInt,
			Default:     "5",
			Value:       fmt.Sprintf("%d", limit),
			Description: "Max search hits to enrich via /t.php (1–25). Ignored when enrich_details is false.",
			Required:    false,
			Group:       "Search",
		},
	}
}

func (m *Module) updateSetting(key, value string) error {
	switch key {
	case "base_url", "PIRATEBAY_API_BASE":
		value = strings.TrimRight(strings.TrimSpace(value), "/")
		if value != "" && !m.fixtureMode && !m.httpInjected {
			if err := enforceLiveIndexerVPN(m.wgConfPath, value, false, false); err != nil {
				return err
			}
			bound, iface, err := newVPNBoundHTTPClient(m.wgConfPath, 20*time.Second)
			if err != nil {
				return fmt.Errorf("vpn-bound HTTP client: %w", err)
			}
			m.mu.Lock()
			m.baseURL = value
			m.http = bound
			m.vpnIface = iface
			m.api = newApibayClient(m.baseURL, bound, m.version)
			m.mu.Unlock()
			return nil
		}
		m.mu.Lock()
		m.baseURL = value
		if !m.fixtureMode {
			m.api = newApibayClient(m.baseURL, m.http, m.version)
		}
		m.mu.Unlock()
		return nil
	case "enrich_details", "PIRATEBAY_ENRICH_DETAILS":
		m.mu.Lock()
		m.enrich = envTruthy(value)
		m.mu.Unlock()
		return nil
	case "enrich_limit", "PIRATEBAY_ENRICH_LIMIT":
		n, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || n < 1 {
			return fmt.Errorf("enrich_limit must be a positive integer")
		}
		if n > 25 {
			n = 25
		}
		m.mu.Lock()
		m.enrichLimit = n
		m.mu.Unlock()
		return nil
	default:
		return fmt.Errorf("unknown setting %q", key)
	}
}

func (m *Module) Settings() []contracts.SettingDef {
	return m.settingsDefs()
}

func (m *Module) UpdateSetting(key, value string) error {
	return m.updateSetting(key, value)
}

func (m *Module) registerSettingsMesh(srv *grpc.Server) {
	modulesdk.RegisterSettings(srv, m.id, m)
}
