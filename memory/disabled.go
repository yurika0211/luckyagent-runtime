package memory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/yurika0211/luckyagent-tools/memory"
	"github.com/yurika0211/luckyagent-tools/memory/diagnostic"
)

const (
	BackendAestus   = "aestus"
	BackendOff      = "off"
	BackendDisabled = "disabled"
)

// NormalizeBackend maps config values. Empty means aestus (current default).
func NormalizeBackend(raw string) (string, error) {
	switch raw {
	case "", BackendAestus:
		return BackendAestus, nil
	case BackendOff, BackendDisabled, "none", "false":
		return BackendOff, nil
	default:
		return "", fmt.Errorf("unknown memory.backend %q (use aestus or off)", raw)
	}
}

// Disabled is an in-process store: no disk, no Aestus SDK.
type Disabled struct{}

var _ memory.RuntimeStore = Disabled{}

func (Disabled) Source() memory.SourceInfo {
	return memory.SourceInfo{Kind: "disabled", Location: ""}
}
func (Disabled) Recall(context.Context, memory.RecallRequest) ([]memory.SearchResult, error) {
	return nil, nil
}
func (Disabled) Remember(context.Context, memory.RememberRequest) (memory.RememberResult, error) {
	return memory.RememberResult{}, nil
}
func (Disabled) AnalyzeMemoryContent(string) []memory.HygieneIssue { return nil }
func (Disabled) Count() int                                        { return 0 }
func (Disabled) Stats() map[memory.Tier]int                        { return map[memory.Tier]int{} }
func (Disabled) Get(string) (*memory.Entry, error)                 { return nil, fmt.Errorf("memory disabled") }
func (Disabled) Recent(int) []memory.Entry                         { return nil }
func (Disabled) GraphTopology(memory.GraphTopologyOptions) memory.GraphTopology {
	return memory.GraphTopology{}
}
func (Disabled) BuildSearchTrace(string, string, string, memory.SearchOptions, []memory.SearchResult, time.Duration) memory.SearchTrace {
	return memory.SearchTrace{}
}
func (Disabled) AuditHygiene(memory.HygieneOptions) memory.HygieneReport { return memory.HygieneReport{} }
func (Disabled) QuarantineDirty(memory.HygieneOptions) (memory.HygieneReport, error) {
	return memory.HygieneReport{}, nil
}
func (Disabled) DeleteDirty(memory.HygieneOptions) (memory.HygieneReport, error) {
	return memory.HygieneReport{}, nil
}
func (Disabled) RestoreHygiene(memory.HygieneOptions) (memory.HygieneReport, error) {
	return memory.HygieneReport{}, nil
}
func (Disabled) ByTierWithOptions(memory.Tier, memory.RouteOptions) []memory.Entry { return nil }
func (Disabled) RouteWithOptions(string, memory.RouteOptions) memory.RouteAnalysis {
	return memory.RouteAnalysis{}
}
func (Disabled) RecordActivationFeedback(string, []memory.Entry, string, float64, time.Time) {}
func (Disabled) RecordTurn(string) memory.MaintenanceEvent                                   { return memory.MaintenanceEvent{} }
func (Disabled) ForgetSession(string)                                                        {}
func (Disabled) Decay(float64) int                                                           { return 0 }
func (Disabled) Expire() int                                                                 { return 0 }
func (Disabled) Promote(string) error                                                        { return nil }
func (Disabled) CompactShortTerm(context.Context, int, int) (int, error)                     { return 0, nil }
func (Disabled) TidalStats() (diagnostic.TidalStoreStats, bool, error) {
	return diagnostic.TidalStoreStats{}, false, nil
}
func (Disabled) Close() error { return nil }

type disabledBuffer struct {
	mu    sync.Mutex
	max   int
	turns []memory.Message
}

func (b *disabledBuffer) Add(role, content string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.turns = append(b.turns, memory.Message{Role: role, Content: content})
	if limit := b.max * 2; limit > 0 && len(b.turns) > limit {
		b.turns = b.turns[len(b.turns)-limit:]
	}
}
func (b *disabledBuffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.turns = nil
}
func (b *disabledBuffer) Compress() {}
func (b *disabledBuffer) GetContext() []memory.Message {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]memory.Message, len(b.turns))
	copy(out, b.turns)
	return out
}
func (b *disabledBuffer) MaxTurns() int { return b.max }
func (b *disabledBuffer) MessageCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.turns)
}
func (b *disabledBuffer) String() string  { return "" }
func (b *disabledBuffer) Summary() string { return "" }

type disabledShortTerm struct {
	mu      sync.Mutex
	max     int
	buffers map[string]*disabledBuffer
}

func newDisabledShortTerm(maxTurns int) *disabledShortTerm {
	if maxTurns <= 0 {
		maxTurns = 10
	}
	return &disabledShortTerm{max: maxTurns, buffers: map[string]*disabledBuffer{}}
}

func (s *disabledShortTerm) buf(sessionID string) *disabledBuffer {
	buf, ok := s.buffers[sessionID]
	if !ok {
		buf = &disabledBuffer{max: s.max}
		s.buffers[sessionID] = buf
	}
	return buf
}
func (s *disabledShortTerm) Add(sessionID, role, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buf(sessionID).Add(role, content)
}
func (s *disabledShortTerm) Buffer(sessionID string) memory.ShortTermBuffer {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf(sessionID)
}
func (s *disabledShortTerm) Clear(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.buffers, sessionID)
}
func (s *disabledShortTerm) Get(sessionID string) (memory.ShortTermBuffer, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	buf, ok := s.buffers[sessionID]
	if !ok {
		return nil, false
	}
	return buf, true
}
func (s *disabledShortTerm) GetContext(sessionID string) []memory.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	if buf, ok := s.buffers[sessionID]; ok {
		return buf.GetContext()
	}
	return nil
}
func (s *disabledShortTerm) SessionCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.buffers)
}
func (s *disabledShortTerm) Summary(string) string { return "" }

type disabledSummaries struct {
	mu      sync.Mutex
	byID    map[string]memory.SessionSummary
	max     int
}

func newDisabledSummaries(max int) *disabledSummaries {
	if max <= 0 {
		max = 100
	}
	return &disabledSummaries{byID: map[string]memory.SessionSummary{}, max: max}
}

func (s *disabledSummaries) SaveSummary(ctx context.Context, r memory.SummaryRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	sum := memory.SessionSummary{SessionID: r.SessionID, UserID: r.UserID, RawSummary: r.Content}
	if r.Title != "" {
		sum.Topics = []string{r.Title}
	}
	return s.SaveSessionSummary(&sum)
}
func (s *disabledSummaries) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.byID)
}
func (s *disabledSummaries) Delete(sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.byID, sessionID)
	return nil
}
func (s *disabledSummaries) ExpireOldSummaries(time.Duration) int { return 0 }
func (s *disabledSummaries) Get(sessionID string) (*memory.SessionSummary, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sum, ok := s.byID[sessionID]
	if !ok {
		return nil, false
	}
	cp := sum
	return &cp, true
}
func (s *disabledSummaries) ListAll() []memory.SessionSummary {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]memory.SessionSummary, 0, len(s.byID))
	for _, v := range s.byID {
		out = append(out, v)
	}
	return out
}
func (s *disabledSummaries) SaveSessionSummary(summary *memory.SessionSummary) error {
	if summary == nil || summary.SessionID == "" {
		return fmt.Errorf("summary required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byID[summary.SessionID] = *summary
	return nil
}
func (s *disabledSummaries) SearchSummaries(string, int) []memory.SessionSummary { return nil }

func DisabledRuntime(shortTermMaxTurns, midTermMaxSummaries int) memory.Runtime {
	return memory.Runtime{
		Store:     Disabled{},
		ShortTerm: newDisabledShortTerm(shortTermMaxTurns),
		Summaries: newDisabledSummaries(midTermMaxSummaries),
	}
}
