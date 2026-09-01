package internal

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Muxcore-Media/core/pkg/contracts"
)

func settingValue(defs []contracts.SettingDef, key string) string {
	for _, d := range defs {
		if d.Key == key {
			return d.Value
		}
	}
	return ""
}

func TestUpdateSettingBaseURLRequiresWGConf(t *testing.T) {
	m := NewModule(Config{GRPCAddr: ":0"})
	if err := m.UpdateSetting("base_url", "https://apibay.example.test"); err == nil {
		t.Fatal("expected WG_CONF error without VPN gate")
	} else if !strings.Contains(err.Error(), "WG_CONF") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateSettingEnrichLimitValidation(t *testing.T) {
	m := NewModule(Config{GRPCAddr: ":0"})
	if err := m.UpdateSetting("enrich_limit", "0"); err == nil {
		t.Fatal("expected error for enrich_limit=0")
	}
	if err := m.UpdateSetting("enrich_limit", "99"); err != nil {
		t.Fatal(err)
	}
	m.mu.RLock()
	got := m.enrichLimit
	m.mu.RUnlock()
	if got != 25 {
		t.Fatalf("enrich_limit=%d want clamped 25", got)
	}
}

func TestUpdateSettingUnknownKey(t *testing.T) {
	m := NewModule(Config{GRPCAddr: ":0"})
	if err := m.UpdateSetting("not_a_key", "x"); err == nil {
		t.Fatal("expected unknown setting error")
	} else if !strings.Contains(err.Error(), "unknown setting") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSettingsReflectSuccessfulUpdate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("[]"))
	}))
	t.Cleanup(srv.Close)

	m := NewModule(Config{
		GRPCAddr: ":0",
		HTTP:     srv.Client(),
	})
	if err := m.UpdateSetting("base_url", srv.URL); err != nil {
		t.Fatal(err)
	}
	if err := m.UpdateSetting("enrich_details", "true"); err != nil {
		t.Fatal(err)
	}
	if err := m.UpdateSetting("enrich_limit", "7"); err != nil {
		t.Fatal(err)
	}

	defs := m.Settings()
	if got := settingValue(defs, "base_url"); got != strings.TrimRight(srv.URL, "/") {
		t.Fatalf("base_url=%q", got)
	}
	if got := settingValue(defs, "enrich_details"); got != "true" {
		t.Fatalf("enrich_details=%q", got)
	}
	if got := settingValue(defs, "enrich_limit"); got != "7" {
		t.Fatalf("enrich_limit=%q", got)
	}
}
