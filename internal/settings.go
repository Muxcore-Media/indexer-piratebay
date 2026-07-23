package internal

import (
	"fmt"
	"strings"

	"github.com/Muxcore-Media/core/pkg/contracts"
	modulesdk "github.com/Muxcore-Media/core/sdk/go/module"
	"google.golang.org/grpc"
)

func (m *Module) settingsDefs() []contracts.SettingDef {
	m.mu.RLock()
	base := m.baseURL
	m.mu.RUnlock()
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
	}
}

func (m *Module) updateSetting(key, value string) error {
	switch key {
	case "base_url", "PIRATEBAY_API_BASE":
		value = strings.TrimRight(strings.TrimSpace(value), "/")
		m.mu.Lock()
		m.baseURL = value
		m.api = newApibayClient(m.baseURL, m.http)
		m.mu.Unlock()
		return nil
	default:
		return fmt.Errorf("unknown setting %q", key)
	}
}

func (m *Module) registerSettingsMesh(srv *grpc.Server) {
	modulesdk.RegisterMeshHandler(srv, m.id, modulesdk.SettingsHandler{
		List:   m.settingsDefs,
		Update: m.updateSetting,
	})
}
