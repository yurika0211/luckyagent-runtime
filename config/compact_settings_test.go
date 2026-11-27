package config

import (
	"encoding/json"
	"testing"
)

// #531 gap 1: the six compaction keys were outside the formal ContextConfig,
// so `lh config set context.compact_keep_tokens 20000` wrote into Extra and
// nothing in the compaction path could see it.
func TestDefaultConfigCarriesCompactKeys(t *testing.T) {
	got := DefaultConfig().Context.CompactSettings()

	if !got.Background {
		t.Fatalf("compact_background default = false, want true")
	}
	if got.PrecomputeLeadTokens != 60000 {
		t.Fatalf("compact_precompute_lead_tokens default = %d, want 60000", got.PrecomputeLeadTokens)
	}
	if got.PrecomputeMaxFailures != 2 {
		t.Fatalf("compact_precompute_max_failures default = %d, want 2", got.PrecomputeMaxFailures)
	}
	if got.CacheMarginSeconds != 30 {
		t.Fatalf("compact_cache_margin_seconds default = %d, want 30", got.CacheMarginSeconds)
	}
	if got.KeepTokens != 15000 {
		t.Fatalf("compact_keep_tokens default = %d, want 15000", got.KeepTokens)
	}
	if !got.ChunkFallback {
		t.Fatalf("compact_chunk_fallback default = false, want true")
	}
}

func TestParseConfigDataReadsCompactKeys(t *testing.T) {
	cfg, err := parseConfigData([]byte(`{"context":{"compact_background":false,"compact_keep_tokens":20000,"compact_chunk_fallback":false,"compact_cache_margin_seconds":90}}`))
	if err != nil {
		t.Fatalf("parseConfigData: %v", err)
	}
	got := cfg.Context.CompactSettings()

	if got.Background {
		t.Fatalf("compact_background = true, want explicit false to survive")
	}
	if got.ChunkFallback {
		t.Fatalf("compact_chunk_fallback = true, want explicit false to survive")
	}
	if got.KeepTokens != 20000 {
		t.Fatalf("compact_keep_tokens = %d, want 20000", got.KeepTokens)
	}
	if got.CacheMarginSeconds != 90 {
		t.Fatalf("compact_cache_margin_seconds = %d, want 90", got.CacheMarginSeconds)
	}
	// Unset neighbours still fall back to defaults.
	if got.PrecomputeLeadTokens != 60000 || got.PrecomputeMaxFailures != 2 {
		t.Fatalf("unset keys = %d/%d, want 60000/2", got.PrecomputeLeadTokens, got.PrecomputeMaxFailures)
	}
}

func TestCompactKeysRoundTripThroughJSON(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Context.CompactBackground = boolPtr(false)
	cfg.Context.CompactKeepTokens = 20000

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	back, err := parseConfigData(data)
	if err != nil {
		t.Fatalf("parseConfigData: %v", err)
	}
	got := back.Context.CompactSettings()
	if got.Background {
		t.Fatalf("compact_background lost its false value across a save/load round trip")
	}
	if got.KeepTokens != 20000 {
		t.Fatalf("compact_keep_tokens = %d after round trip, want 20000", got.KeepTokens)
	}
}

func TestManagerSetWritesCompactKeysIntoContext(t *testing.T) {
	m := &Manager{config: DefaultConfig()}

	for key, value := range map[string]string{
		"context.compact_keep_tokens":             "20000",
		"context.compact_precompute_lead_tokens":  "80000",
		"context.compact_precompute_max_failures": "3",
		"context.compact_cache_margin_seconds":    "45",
		"context.compact_background":              "false",
		"context.compact_chunk_fallback":          "no",
	} {
		if err := m.Set(key, value); err != nil {
			t.Fatalf("Set(%s): %v", key, err)
		}
	}

	got := m.config.Context.CompactSettings()
	if got.KeepTokens != 20000 {
		t.Fatalf("compact_keep_tokens = %d, want 20000", got.KeepTokens)
	}
	if got.PrecomputeLeadTokens != 80000 {
		t.Fatalf("compact_precompute_lead_tokens = %d, want 80000", got.PrecomputeLeadTokens)
	}
	if got.PrecomputeMaxFailures != 3 {
		t.Fatalf("compact_precompute_max_failures = %d, want 3", got.PrecomputeMaxFailures)
	}
	if got.CacheMarginSeconds != 45 {
		t.Fatalf("compact_cache_margin_seconds = %d, want 45", got.CacheMarginSeconds)
	}
	if got.Background {
		t.Fatalf("compact_background = true, want false")
	}
	if got.ChunkFallback {
		t.Fatalf("compact_chunk_fallback = true, want false")
	}

	// The whole point of the fix: these keys must not land in the catch-all map.
	for key := range m.config.Extra {
		if len(key) >= len("context.compact_") && key[:len("context.compact_")] == "context.compact_" {
			t.Fatalf("%s still fell through to Extra", key)
		}
	}
}

func TestCompactSettingsIgnoresZeroValueStruct(t *testing.T) {
	got := ContextConfig{}.CompactSettings()
	if !got.Background || !got.ChunkFallback {
		t.Fatalf("zero value = %+v, want default-true bools", got)
	}
	if got.KeepTokens != 15000 {
		t.Fatalf("zero value keep tokens = %d, want 15000", got.KeepTokens)
	}
}