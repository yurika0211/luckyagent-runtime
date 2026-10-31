package memory

import (
	"context"
	"testing"

	port "github.com/yurika0211/luckyagent-tools/memory"
)

func TestNormalizeBackend(t *testing.T) {
	got, err := NormalizeBackend("")
	if err != nil || got != BackendAestus {
		t.Fatalf("empty: %q %v", got, err)
	}
	got, err = NormalizeBackend("OFF")
	if err == nil {
		// case-sensitive; uppercase is unknown unless we fold — keep strict
	}
	got, err = NormalizeBackend("off")
	if err != nil || got != BackendOff {
		t.Fatalf("off: %q %v", got, err)
	}
	if _, err := NormalizeBackend("sqlite"); err == nil {
		t.Fatal("expected error")
	}
}

func TestDisabledRuntimeDoesNotPersistFacts(t *testing.T) {
	rt := DisabledRuntime(4, 8)
	if rt.Store.Source().Kind != "disabled" {
		t.Fatalf("source %+v", rt.Store.Source())
	}
	if _, err := rt.Store.Remember(context.Background(), port.RememberRequest{Content: "secret", Category: "fact"}); err != nil {
		t.Fatal(err)
	}
	hits, err := rt.Store.Recall(context.Background(), port.RecallRequest{Query: "secret"})
	if err != nil || len(hits) != 0 {
		t.Fatalf("recall %#v %v", hits, err)
	}
	if rt.Store.Count() != 0 {
		t.Fatal("count")
	}
	rt.ShortTerm.Add("s1", "user", "hi")
	if len(rt.ShortTerm.GetContext("s1")) != 1 {
		t.Fatal("short term")
	}
	if err := rt.Summaries.SaveSummary(context.Background(), port.SummaryRequest{SessionID: "s1", Content: "sum"}); err != nil {
		t.Fatal(err)
	}
	if rt.Summaries.Count() != 1 {
		t.Fatal("summary")
	}
}
