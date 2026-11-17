package contextx

import (
	"hash/fnv"
	"math"
	"sync"
)

// --- Token 估算器 ---
//
// 精确 tokenizer 需要模型词表，本估算器不引入该依赖，也不承诺相对真实
// provider usage 的误差范围。Estimate 按 rune 类别累加固定正 cost units，
// 最后一次 ceil。类别权重不随整段 CJK/code 比例切换，因此在文本前、中、后
// 插入内容不会把更长字符串估得更低。
//
// 权重对齐旧的单类别除数，只作为上下文窗口和请求预算的启发式：
//   - CJK:  2/3 unit/rune（旧中文约 1.5 chars/token）
//   - code: 1/3 unit/rune（旧代码约 3 chars/token）
//   - 其余: 1/4 unit/rune（旧英文约 4 chars/token）
//
// 纯英文、纯 CJK、纯 code 与旧公式同量级。混合文本不再在 30% CJK、5% CJK、
// 15% code 阈值两侧跳变，消费侧必须用新估值重跑 auto 阈值和请求预算。
// 估值不是 provider token，也不能当成线上省 token 的证据。

// ContentType 表示文本内容类型
type ContentType int

const (
	ContentAuto    ContentType = iota // 自动检测
	ContentEnglish                    // 英文文本
	ContentChinese                    // 中文文本
	ContentCode                       // 代码
	ContentMixed                      // 混合内容
)

// TokenEstimator 估算文本的 token 数量
type TokenEstimator struct {
	modelContextWindow int // 模型上下文窗口大小
	mu                 sync.RWMutex
	cache              map[uint64]int
}

// NewTokenEstimator 创建 Token 估算器
func NewTokenEstimator(modelContextWindow int) *TokenEstimator {
	if modelContextWindow <= 0 {
		modelContextWindow = 4096 // 默认
	}
	return &TokenEstimator{
		modelContextWindow: modelContextWindow,
		cache:              make(map[uint64]int),
	}
}

// Estimate 估算文本的 token 数量
func (te *TokenEstimator) Estimate(text string) int {
	if text == "" {
		return 0
	}

	key := tokenEstimateCacheKey(text)
	te.mu.RLock()
	if cached, ok := te.cache[key]; ok {
		te.mu.RUnlock()
		return cached
	}
	te.mu.RUnlock()

	// 固定正类别权重，取整前可加。不按整段比例切换除数。
	// 消息 role/name/tool 开销仍在 EstimateMessage 中单独加。
	estimated := estimateTokensMonotonic(text)
	te.mu.Lock()
	// 简单上限，避免无界增长
	if len(te.cache) > 4096 {
		te.cache = make(map[uint64]int, 2048)
	}
	te.cache[key] = estimated
	te.mu.Unlock()
	return estimated
}

// EstimateMessages 估算消息列表的总 token 数量
func (te *TokenEstimator) EstimateMessages(messages []Message) int {
	total := 0
	for _, msg := range messages {
		total += te.EstimateMessage(msg)
	}
	// 对话模板开销（system 格式化等）
	total += 4 // <|im_start|> 和 <|im_end|> 等
	return total
}

// EstimateMessage 估算单条消息的 token 数量
func (te *TokenEstimator) EstimateMessage(msg Message) int {
	// 消息开销：role 标记 + 格式化 ≈ 4 tokens
	overhead := 4

	// 如果有 name 字段，额外 +1
	if msg.Name != "" {
		overhead += 1 + te.Estimate(msg.Name)
	}

	contentTokens := te.Estimate(msg.Content)
	if msg.ReasoningContent != "" {
		contentTokens += te.Estimate(msg.ReasoningContent)
	}

	// 工具调用额外开销
	if len(msg.ToolCalls) > 0 {
		for _, tc := range msg.ToolCalls {
			contentTokens += te.Estimate(tc.Name) + te.Estimate(tc.Arguments) + 4
		}
	}

	return overhead + contentTokens
}

// ModelContextWindow 返回模型的上下文窗口大小
func (te *TokenEstimator) ModelContextWindow() int {
	return te.modelContextWindow
}

// SetModelContextWindow 设置模型上下文窗口大小
func (te *TokenEstimator) SetModelContextWindow(window int) {
	if window > 0 {
		te.mu.Lock()
		te.modelContextWindow = window
		te.mu.Unlock()
	}
}

// RemainingTokens 计算剩余可用 token 数量
func (te *TokenEstimator) RemainingTokens(messages []Message) int {
	used := te.EstimateMessages(messages)
	te.mu.RLock()
	window := te.modelContextWindow
	te.mu.RUnlock()
	remaining := window - used
	if remaining < 0 {
		return 0
	}
	return remaining
}

// UsagePercent 计算上下文使用百分比
func (te *TokenEstimator) UsagePercent(messages []Message) float64 {
	used := te.EstimateMessages(messages)
	return float64(used) / float64(te.modelContextWindow) * 100
}

// estimateTokensMonotonic sums fixed positive per-rune costs and ceils once.
// Empty text is 0. Adding or inserting runes cannot decrease the result,
// aside from the plateau caused by a single ceil.
func estimateTokensMonotonic(text string) int {
	if text == "" {
		return 0
	}
	var units float64
	for _, r := range text {
		switch {
		case isCJK(r):
			units += tokenCostCJK
		case isCodeChar(r):
			units += tokenCostCode
		default:
			units += tokenCostOther
		}
	}
	if units <= 0 {
		return 0
	}
	return int(math.Ceil(units))
}

// Per-rune cost units. 1 unit is 1 estimated token before ceil.
// Values match the historical single-class chars/token divisors.
const (
	tokenCostCJK   = 2.0 / 3.0
	tokenCostCode  = 1.0 / 3.0
	tokenCostOther = 1.0 / 4.0
)

// --- 辅助函数 ---

// detectContentType 检测文本的主要语言/类型。
// Estimate 不再使用它选择除数；保留给既有内容类型测试和调用方。
func detectContentType(text string) ContentType {
	if text == "" {
		return ContentMixed
	}

	cjkCount := 0
	latinCount := 0
	codeCount := 0
	total := 0

	for _, r := range text {
		total++
		switch {
		case isCJK(r):
			cjkCount++
		case isCodeChar(r):
			codeCount++
		default:
			latinCount++
		}
	}

	if total == 0 {
		return ContentMixed
	}

	cjkRatio := float64(cjkCount) / float64(total)
	codeRatio := float64(codeCount) / float64(total)

	if cjkRatio > 0.3 {
		return ContentChinese
	}
	if codeRatio > 0.15 {
		return ContentCode
	}
	if cjkRatio > 0.05 {
		return ContentMixed
	}
	return ContentEnglish
}

// charsPerToken 根据内容类型返回每 token 的字符数
func charsPerToken(ct ContentType) float64 {
	switch ct {
	case ContentEnglish:
		return 4.0 // 英文约 4 字符/token
	case ContentChinese:
		return 1.5 // 中文约 1.5 字符/token
	case ContentCode:
		return 3.0 // 代码约 3 字符/token
	case ContentMixed:
		return 2.5 // 混合内容取中间值
	default:
		return 3.0
	}
}

// isCJK 判断是否是 CJK 字符
func isCJK(r rune) bool {
	return (r >= 0x4E00 && r <= 0x9FFF) || // CJK Unified Ideographs
		(r >= 0x3400 && r <= 0x4DBF) || // CJK Extension A
		(r >= 0x3000 && r <= 0x303F) || // CJK Symbols and Punctuation
		(r >= 0x3040 && r <= 0x309F) || // Hiragana
		(r >= 0x30A0 && r <= 0x30FF) || // Katakana
		(r >= 0xAC00 && r <= 0xD7AF) // Korean Syllables
}

// isCodeChar 判断是否是代码字符（大括号、分号等）
func isCodeChar(r rune) bool {
	return r == '{' || r == '}' || r == '(' || r == ')' ||
		r == '[' || r == ']' || r == ';' || r == '=' ||
		r == '<' || r == '>' || r == '/' || r == '\\' ||
		r == '&' || r == '|' || r == '!' || r == '@' ||
		r == '#' || r == '$' || r == '%' || r == '^' ||
		r == '*' || r == '~' || r == '`'
}

func tokenEstimateCacheKey(text string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(text))
	return h.Sum64()
}
