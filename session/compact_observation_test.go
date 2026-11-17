package session

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/yurika0211/luckyagent-runtime/provider"
)

func TestCompactObservationRoundTripSaveReloadAndLatestTrace(t *testing.T) {
	dir := t.TempDir()
	s := NewSession("compact-obs", dir)
	s.AddMessage("user", "old request")
	s.AddMessage("assistant", "old answer")
	created := time.Date(2026, 11, 17, 3, 4, 5, 0, time.UTC)
	usage := &provider.TokenUsage{
		InputTokens:       1200,
		OutputTokens:      80,
		TotalTokens:       1280,
		CachedInputTokens: 100,
		Model:             "example-model",
	}
	s.AddCompactBoundary(CompactMetadata{
		ID:                   "compact-obs-1",
		Trigger:              "manual",
		Summary:              "kept summary",
		FromMessage:          0,
		ToMessage:            2,
		CreatedAt:            created,
		PreTokenEstimate:     400,
		PostTokenEstimate:    90,
		SummaryTokens:        90,
		DroppedMessages:      2,
		RetainedMessages:     0,
		SummarySource:        "llm",
		SummaryUsage:         usage,
		SummaryUsageRecords:  1,
		SummaryUsageComplete: true,
		SummaryCalls:         1,
		SummaryDurationMS:    9100,
		SummarySegments:      1,
	})
	s.AddMessage("user", "tail request")
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	mgr, err := NewManager(dir)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	loaded, ok := mgr.Get("compact-obs")
	if !ok {
		t.Fatal("session missing after reload")
	}
	_, meta, covered, found := MessagesAfterLastCompactBoundary(loaded.GetMessages())
	if !found || covered != 2 {
		t.Fatalf("reload boundary missing: found=%v covered=%d meta=%+v", found, covered, meta)
	}
	assertObservation(t, "reloaded-meta", meta)
	trace, ok := loaded.LatestCompactTrace()
	if !ok {
		t.Fatal("latest trace missing after reload")
	}
	assertTraceObservation(t, trace)
	if trace.SummaryUsage == meta.SummaryUsage {
		t.Fatal("trace must clone usage instead of aliasing metadata")
	}
	trace.SummaryUsage.InputTokens = 1
	if meta.SummaryUsage.InputTokens != 1200 {
		t.Fatal("trace usage alias mutated metadata")
	}

	undone, err := loaded.UndoLatestCompactBoundary(true)
	if err != nil {
		t.Fatalf("undo: %v", err)
	}
	assertObservation(t, "undo", undone)
}

func TestCompactObservationMissingUsageStaysDistinctFromZero(t *testing.T) {
	created := time.Date(2026, 11, 17, 1, 0, 0, 0, time.UTC)
	meta := CompactMetadata{
		ID:                "compact-missing-usage",
		Trigger:           "manual",
		Summary:           "summary without provider usage",
		FromMessage:       0,
		ToMessage:         1,
		CreatedAt:         created,
		SummarySource:     "llm",
		SummaryCalls:      1,
		SummaryDurationMS: 2200,
		SummarySegments:   1,
	}
	msg := CompactBoundaryMessage(meta)
	if strings.Contains(msg.Content, "summary_usage") || strings.Contains(msg.Content, "summary_ttft") {
		t.Fatalf("missing usage must not invent usage or ttft: %s", msg.Content)
	}
	parsed, ok := ParseCompactMetadata(msg)
	if !ok {
		t.Fatal("parse failed")
	}
	if parsed.SummaryUsage != nil || parsed.SummaryUsageRecords != 0 || parsed.SummaryUsageComplete {
		t.Fatalf("missing usage collapsed into a zero record: %+v", parsed.SummaryUsage)
	}
	if parsed.SummaryCalls != 1 || parsed.SummaryDurationMS != 2200 || parsed.SummarySegments != 1 {
		t.Fatalf("host-observed fields dropped: %+v", parsed)
	}
	trace := CompactTraceFromMetadata(parsed)
	if trace.SummaryUsage != nil || trace.SummaryCalls != 1 || trace.SummarySegments != 1 {
		t.Fatalf("trace dropped missing-usage distinction: %+v", trace)
	}

	zero := meta
	zero.ID = "compact-explicit-zero"
	zero.SummaryUsage = &provider.TokenUsage{}
	zero.SummaryUsageRecords = 1
	zeroMsg := CompactBoundaryMessage(zero)
	if !strings.Contains(zeroMsg.Content, `"summary_usage"`) {
		t.Fatalf("explicit zero usage must be stored: %s", zeroMsg.Content)
	}
	parsedZero, ok := ParseCompactMetadata(zeroMsg)
	if !ok || parsedZero.SummaryUsage == nil || parsedZero.SummaryUsageRecords != 1 {
		t.Fatalf("explicit zero usage lost: %+v ok=%v", parsedZero, ok)
	}
}

func TestCompactLegacyBoundaryOmitsObservation(t *testing.T) {
	legacyJSON := `{"id":"legacy-json","trigger":"manual","summary":"old summary","created_at":"2026-01-02T03:04:05Z","summary_source":"llm"}`
	parsed, ok := ParseCompactMetadata(provider.Message{
		Role:    "system",
		Name:    CompactBoundaryName,
		Content: legacyJSON,
	})
	if !ok {
		t.Fatal("legacy json should parse")
	}
	if parsed.SummaryUsage != nil || parsed.SummaryCalls != 0 || parsed.SummaryDurationMS != 0 || parsed.SummarySegments != 0 || parsed.SummaryUsageComplete {
		t.Fatalf("legacy json gained observation: %+v", parsed)
	}
	if parsed.SummarySource != "llm" || parsed.Summary != "old summary" {
		t.Fatalf("legacy fields changed: %+v", parsed)
	}
	trace := CompactTraceFromMetadata(parsed)
	encoded, err := json.Marshal(trace)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "summary_usage") || strings.Contains(string(encoded), "summary_calls") {
		t.Fatalf("legacy trace emitted new keys: %s", encoded)
	}

	plain, ok := ParseCompactMetadata(provider.Message{
		Role:    "system",
		Name:    CompactBoundaryName,
		Content: "plain legacy summary",
	})
	if !ok || plain.Summary != "plain legacy summary" || plain.SummaryUsage != nil || plain.SummaryCalls != 0 {
		t.Fatalf("plain legacy boundary changed: %+v ok=%v", plain, ok)
	}

	dir := t.TempDir()
	s := NewSession("legacy-compact", dir)
	s.AddMessage("user", "kept raw")
	s.AddCompactBoundary(CompactMetadata{ID: "legacy-only", Summary: "legacy summary"})
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	mgr, err := NewManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	loaded, ok := mgr.Get("legacy-compact")
	if !ok {
		t.Fatal("missing reloaded legacy session")
	}
	trace, ok = loaded.LatestCompactTrace()
	if !ok || trace.BoundaryID != "legacy-only" || trace.SummaryUsage != nil || trace.SummaryCalls != 0 {
		t.Fatalf("reloaded legacy trace = %+v ok=%v", trace, ok)
	}
}

func TestCompactObservationSurvivesBackupRestore(t *testing.T) {
	dir := t.TempDir()
	s := NewSession("compact-backup", dir)
	s.AddMessage("user", "before compact")
	s.AddCompactBoundary(CompactMetadata{
		ID:                  "backup-boundary",
		Trigger:             "manual",
		Summary:             "backup summary",
		FromMessage:         0,
		ToMessage:           1,
		CreatedAt:           time.Date(2026, 11, 17, 8, 0, 0, 0, time.UTC),
		SummarySource:       "llm",
		SummaryCalls:        2,
		SummaryUsageRecords: 1,
		SummaryUsage: &provider.TokenUsage{
			InputTokens:  10,
			OutputTokens: 4,
			TotalTokens:  14,
			Model:        "backup-model",
		},
		SummaryDurationMS: 1500,
		SummarySegments:   1,
	})
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := s.CreateBackup("manual")
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}
	s.AddMessage("user", "after backup")
	if err := s.Save(); err != nil {
		t.Fatalf("Save after: %v", err)
	}
	if _, err := s.RestoreBackup(info.ID); err != nil {
		t.Fatalf("RestoreBackup: %v", err)
	}
	_, meta, _, ok := MessagesAfterLastCompactBoundary(s.GetMessages())
	if !ok || meta.SummaryCalls != 2 || meta.SummaryUsage == nil || meta.SummaryUsage.Model != "backup-model" || meta.SummaryUsageRecords != 1 {
		t.Fatalf("backup restore lost observation: %+v ok=%v", meta, ok)
	}
	if meta.SummaryUsageComplete {
		t.Fatal("incomplete usage must stay incomplete across backup")
	}
}

func assertObservation(t *testing.T, label string, meta CompactMetadata) {
	t.Helper()
	if meta.ID != "compact-obs-1" || meta.SummarySource != "llm" {
		t.Fatalf("%s identity: %+v", label, meta)
	}
	if meta.SummaryUsage == nil {
		t.Fatalf("%s usage missing", label)
	}
	if meta.SummaryUsage.InputTokens != 1200 || meta.SummaryUsage.OutputTokens != 80 || meta.SummaryUsage.TotalTokens != 1280 || meta.SummaryUsage.CachedInputTokens != 100 || meta.SummaryUsage.Model != "example-model" {
		t.Fatalf("%s usage: %+v", label, meta.SummaryUsage)
	}
	if meta.SummaryUsageRecords != 1 || !meta.SummaryUsageComplete || meta.SummaryCalls != 1 || meta.SummaryDurationMS != 9100 || meta.SummarySegments != 1 {
		t.Fatalf("%s counts: records=%d complete=%v calls=%d duration=%d segments=%d", label, meta.SummaryUsageRecords, meta.SummaryUsageComplete, meta.SummaryCalls, meta.SummaryDurationMS, meta.SummarySegments)
	}
}

func assertTraceObservation(t *testing.T, trace CompactTrace) {
	t.Helper()
	if trace.BoundaryID != "compact-obs-1" || trace.SummarySource != "llm" {
		t.Fatalf("trace identity: %+v", trace)
	}
	if trace.SummaryUsage == nil || trace.SummaryUsage.InputTokens != 1200 || trace.SummaryUsage.Model != "example-model" {
		t.Fatalf("trace usage: %+v", trace.SummaryUsage)
	}
	if trace.SummaryUsageRecords != 1 || !trace.SummaryUsageComplete || trace.SummaryCalls != 1 || trace.SummaryDurationMS != 9100 || trace.SummarySegments != 1 {
		t.Fatalf("trace counts: %+v", trace)
	}
}
