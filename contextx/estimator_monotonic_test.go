package contextx

import (
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
)

// Weights are fixed positive class costs. These checks cover monotonicity
// across the old 30% CJK, 5% CJK and 15% code classification edges.
// They do not treat the historical 610 / 497 pair as a compatibility target:
// 610 was the old Chinese-class estimate of the 915-rune fixture, and 497
// was the buggy shorter estimate after English wrapping crossed 30% CJK.

func TestEstimateIssue915WrappingDoesNotDecrease(t *testing.T) {
	est := NewTokenEstimator(128000)
	body := syntheticIssue915Body()
	if utf8.RuneCountInString(body) != 915 {
		t.Fatalf("fixture runes = %d, want 915", utf8.RuneCountInString(body))
	}
	cjk := countClass(body, isCJK)
	if cjk != 361 {
		t.Fatalf("fixture CJK = %d, want 361", cjk)
	}
	bare := est.Estimate(body)
	// Old classifier: ceil(915/1.5) = 610. The monotonic estimate is lower
	// because CJK and other runes no longer share the Chinese divisor.
	// 610 is the retired bug baseline, not the value this estimator must keep.
	if bare <= 0 {
		t.Fatalf("bare estimate = %d", bare)
	}
	if bare == 610 {
		t.Logf("bare estimate happened to equal the retired 610 baseline; monotonicity checks still apply")
	}

	wrap := strings.Repeat("w", 317)
	wrapped := wrap + body
	if utf8.RuneCountInString(wrapped) != 1232 {
		t.Fatalf("wrapped runes = %d, want 1232", utf8.RuneCountInString(wrapped))
	}
	got := est.Estimate(wrapped)
	if got < bare {
		t.Fatalf("English wrap decreased estimate: bare=%d wrapped=%d (old bug pair was 610 -> 497)", bare, got)
	}
}

func TestEstimateDoesNotDecreaseAcrossClassThresholds(t *testing.T) {
	est := NewTokenEstimator(128000)
	// Start just over each historical ratio, then add other/code/CJK runes
	// until the ratio falls through the threshold. The longer text must not
	// estimate lower.
	cases := []struct {
		name string
		base string
		add  string
	}{
		{
			name: "cross-30-cjk",
			base: strings.Repeat("中", 31) + strings.Repeat("a", 69), // 31% CJK
			add:  strings.Repeat("a", 40),                           // falls under 30%
		},
		{
			name: "cross-5-cjk",
			base: strings.Repeat("中", 6) + strings.Repeat("a", 94), // 6% CJK
			add:  strings.Repeat("a", 40),                          // falls under 5%
		},
		{
			name: "cross-15-code",
			base: strings.Repeat("{", 16) + strings.Repeat("a", 84), // 16% code
			add:  strings.Repeat("a", 30),                           // falls under 15%
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := est.Estimate(tc.base)
			after := est.Estimate(tc.base + tc.add)
			if after < before {
				t.Fatalf("%s decreased: before=%d after=%d", tc.name, before, after)
			}
			if utf8.RuneCountInString(tc.base+tc.add) <= utf8.RuneCountInString(tc.base) {
				t.Fatalf("%s did not grow", tc.name)
			}
		})
	}
}

func TestEstimateInsertPositionsDoNotDecrease(t *testing.T) {
	est := NewTokenEstimator(8192)
	bases := []string{
		"plain english sentence about budgets",
		strings.Repeat("测", 40) + " tail",
		"func main() { return 1 }",
		"混合 mixed {code} 与 English",
	}
	inserts := []string{
		" WRAP",
		"中",
		"{};",
		"\u00e9\u0301", // combining sequence, still added runes
		" ",
	}
	for _, base := range bases {
		bare := est.Estimate(base)
		runes := []rune(base)
		for _, ins := range inserts {
			positions := []int{0, len(runes) / 2, len(runes)}
			for _, pos := range positions {
				var b strings.Builder
				b.WriteString(string(runes[:pos]))
				b.WriteString(ins)
				b.WriteString(string(runes[pos:]))
				got := est.Estimate(b.String())
				if got < bare {
					t.Fatalf("insert %q at %d decreased %d -> %d for %q", ins, pos, bare, got, base)
				}
			}
		}
	}
}

func TestEstimateEmptyAndCacheRace(t *testing.T) {
	est := NewTokenEstimator(4096)
	if est.Estimate("") != 0 {
		t.Fatal("empty estimate must stay 0")
	}
	text := strings.Repeat("cache-race 中文 {code} ", 32)
	want := est.Estimate(text)
	var wg sync.WaitGroup
	errCh := make(chan string, 64)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 40; j++ {
				if got := est.Estimate(text); got != want {
					errCh <- "cache mismatch"
					return
				}
				if est.Estimate("") != 0 {
					errCh <- "empty mismatch"
					return
				}
				// Distinct suffixes keep the cache lock on the write path.
				_ = est.Estimate(text + strings.Repeat("x", j%7))
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for msg := range errCh {
		t.Fatal(msg)
	}
	if again := est.Estimate(text); again != want {
		t.Fatalf("hot cache = %d, cold/first = %d", again, want)
	}
}

func TestEstimateMessageOverheadStillCompatible(t *testing.T) {
	est := NewTokenEstimator(4096)
	content := "Hello compact budget"
	base := Message{Role: "user", Content: content}
	withName := Message{Role: "user", Content: content, Name: "Ada"}
	withTool := Message{
		Role:    "assistant",
		Content: content,
		ToolCalls: []ToolCall{{
			Name:      "read",
			Arguments: `{"path":"a"}`,
		}},
	}
	contentTokens := est.Estimate(content)
	if got := est.EstimateMessage(base); got != contentTokens+4 {
		t.Fatalf("role overhead = %d, want %d", got, contentTokens+4)
	}
	nameTokens := est.Estimate("Ada")
	if got := est.EstimateMessage(withName); got != contentTokens+4+1+nameTokens {
		t.Fatalf("name overhead = %d, want %d", got, contentTokens+4+1+nameTokens)
	}
	toolTokens := est.Estimate("read") + est.Estimate(`{"path":"a"}`) + 4
	if got := est.EstimateMessage(withTool); got != contentTokens+4+toolTokens {
		t.Fatalf("tool overhead = %d, want %d", got, contentTokens+4+toolTokens)
	}
	list := est.EstimateMessages([]Message{base, withName})
	if list != est.EstimateMessage(base)+est.EstimateMessage(withName)+4 {
		t.Fatalf("list template overhead missing: %d", list)
	}
}

func TestWindowFitHonorsEstimateBudget(t *testing.T) {
	critical := Message{
		Role:     "system",
		Content:  "keep this pinned instruction",
		Priority: PriorityCritical,
	}
	filler := Message{
		Role:     "user",
		Content:  strings.Repeat("droppable history line. ", 40),
		Priority: PriorityLow,
	}
	must := Message{
		Role:     "user",
		Content:  "must-retain-tail",
		Priority: PriorityHigh,
	}
	cfg := WindowConfig{
		MaxTokens:      80,
		ReservedTokens: 8,
		Strategy:       TrimLowPriority,
	}
	cw := NewContextWindow(cfg)
	available := cfg.MaxTokens - cfg.ReservedTokens
	input := []Message{critical, filler, filler, must}
	fitted, result := cw.Fit(input)
	foundCritical := false
	foundMust := false
	for _, msg := range fitted {
		if msg.Priority == PriorityCritical && msg.Content == critical.Content {
			foundCritical = true
		}
		if msg.Content == must.Content {
			foundMust = true
		}
	}
	if !foundCritical {
		t.Fatal("Fit dropped the critical message")
	}
	if !foundMust {
		t.Fatal("Fit dropped the high-priority tail that should fit with the critical message")
	}
	if result.FinalTokens != cw.EstimateTokens(fitted) {
		t.Fatalf("Fit token report %d != re-estimate %d", result.FinalTokens, cw.EstimateTokens(fitted))
	}
	// Critical content is pinned even when it alone exceeds the budget.
	// This case is sized so critical + must fit, and the long filler does not.
	if result.FinalTokens > available {
		t.Fatalf("fitted non-overflowing selection exceeded budget: %d > %d", result.FinalTokens, available)
	}
	if !result.Trimmed {
		t.Fatal("expected the long filler to be trimmed")
	}
}

func syntheticIssue915Body() string {
	// 361 CJK + 554 other = 915 runes. CJK ratio 361/915 = 39.45%, the
	// historical Chinese class. Wrapping with 317 other runes drops it to
	// 29.30%, which used to switch the divisor and shrink the estimate.
	return strings.Repeat("摘要", 180) + "结" + strings.Repeat(" note", 110) + "xxxx"
}

func countClass(text string, pred func(rune) bool) int {
	n := 0
	for _, r := range text {
		if pred(r) {
			n++
		}
	}
	return n
}

func TestSyntheticIssue915BodyShape(t *testing.T) {
	body := syntheticIssue915Body()
	runes := utf8.RuneCountInString(body)
	cjk := countClass(body, isCJK)
	other := runes - cjk - countClass(body, isCodeChar)
	if runes != 915 || cjk != 361 || other != 554 {
		t.Fatalf("shape runes=%d cjk=%d other=%d", runes, cjk, other)
	}
}
