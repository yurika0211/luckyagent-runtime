package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDefaultConfigOmitsRemovedAgentBridges(t *testing.T) {
	data, err := json.Marshal(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, `"codex"`) || strings.Contains(text, `"grok"`) {
		t.Fatalf("default config still serializes a removed bridge: %s", text)
	}
}

func TestParseConfigIgnoresLegacyCodexAndGrokObjects(t *testing.T) {
	cfg, err := parseConfigData([]byte(`{"codex":{"enabled":true,"command":"codex"},"grok":{"enabled":true,"command":"grok"}}`))
	if err != nil {
		t.Fatalf("legacy bridge objects should be ignored, got %v", err)
	}
	if cfg == nil {
		t.Fatal("expected config")
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, `"codex"`) || strings.Contains(text, `"grok"`) {
		t.Fatalf("loaded config kept a removed bridge: %s", text)
	}
}

func TestSetLegacyCodexEnabledFallsThroughToExtra(t *testing.T) {
	mgr, err := NewManagerWithDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.Set("codex.enabled", "true"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got := mgr.Get()
	if got == nil || got.Extra["codex.enabled"] != "true" {
		t.Fatalf("expected extra fallback, got %#v", got)
	}
}
