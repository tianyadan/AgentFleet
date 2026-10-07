package agent

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

// RunMeta 一轮引擎调用的用量/会话元数据。
// UsedTokens 来自本轮 input(+cache)，是估算占用，不是跨 turn 累计，也不等于引擎内部真实 context occupancy。
// Estimated=true 表示展示时须标明「约」；仅 Claude /context probe 成功后才应把对应 EngineContext.Estimated 置 false。
type RunMeta struct {
	SessionID     string `json:"session_id,omitempty"`
	InputTokens   int64  `json:"input_tokens"`
	OutputTokens  int64  `json:"output_tokens"`
	CacheRead     int64  `json:"cache_read_tokens"`
	CacheWrite    int64  `json:"cache_write_tokens"`
	ContextWindow int64  `json:"context_window"`
	UsedTokens    int64  `json:"used_tokens"` // 本轮估算占用（优先 input+cache），勿当精确值
	Estimated     bool   `json:"estimated"`   // stream/JSONL usage 恒为 true
}

// MetaFn 用量回调。
type MetaFn func(RunMeta)

// ParseClaudeStreamMeta 从 stream-json 一行提取 session / usage。
func ParseClaudeStreamMeta(line []byte) (RunMeta, bool) {
	var raw map[string]interface{}
	if json.Unmarshal(line, &raw) != nil {
		return RunMeta{}, false
	}
	var m RunMeta
	ok := false
	if sid := ExtractSessionID(raw); sid != "" {
		m.SessionID = sid
		ok = true
	}
	if u, _ := raw["usage"].(map[string]interface{}); u != nil {
		m.InputTokens = jsonInt64(u["input_tokens"])
		m.OutputTokens = jsonInt64(u["output_tokens"])
		m.CacheRead = jsonInt64(u["cache_read_input_tokens"])
		m.CacheWrite = jsonInt64(u["cache_creation_input_tokens"])
		ok = true
	}
	if mu, _ := raw["modelUsage"].(map[string]interface{}); mu != nil {
		for _, v := range mu {
			mm, _ := v.(map[string]interface{})
			if mm == nil {
				continue
			}
			if cw := jsonInt64(mm["contextWindow"]); cw > 0 {
				m.ContextWindow = cw
				ok = true
			}
			if m.InputTokens == 0 {
				m.InputTokens = jsonInt64(mm["inputTokens"])
			}
			if m.OutputTokens == 0 {
				m.OutputTokens = jsonInt64(mm["outputTokens"])
			}
		}
	}
	if typ, _ := raw["type"].(string); typ == "assistant" {
		if msg, _ := raw["message"].(map[string]interface{}); msg != nil {
			if u, _ := msg["usage"].(map[string]interface{}); u != nil {
				if m.InputTokens == 0 {
					m.InputTokens = jsonInt64(u["input_tokens"])
				}
				if m.OutputTokens == 0 {
					m.OutputTokens = jsonInt64(u["output_tokens"])
				}
				ok = true
			}
		}
	}
	// stream usage = 本轮估算，真值需 /context probe。
	m.UsedTokens = m.InputTokens + m.CacheRead
	if m.UsedTokens == 0 && m.OutputTokens > 0 {
		m.UsedTokens = m.InputTokens + m.OutputTokens
	}
	if ok && (m.InputTokens > 0 || m.OutputTokens > 0 || m.UsedTokens > 0) {
		m.Estimated = true
	}
	return m, ok
}

var (
	reContextTokens = regexp.MustCompile(`(?i)Tokens:\s*([\d.]+)\s*([kKmM]?)\s*/\s*([\d.]+)\s*([kKmM]?)`)
	reContextPct    = regexp.MustCompile(`\((\d+(?:\.\d+)?)\s*%\)`)
)

// ParseClaudeContextText 解析 /context 输出中的 Tokens: 24.1k / 1m。
func ParseClaudeContextText(text string) (used, window int64, ok bool) {
	clean := strings.ReplaceAll(text, "*", "")
	m := reContextTokens.FindStringSubmatch(clean)
	if m == nil {
		return 0, 0, false
	}
	used = scaleTokenNum(m[1], m[2])
	window = scaleTokenNum(m[3], m[4])
	if window <= 0 {
		return 0, 0, false
	}
	return used, window, true
}

func scaleTokenNum(num, suffix string) int64 {
	f, err := strconv.ParseFloat(strings.TrimSpace(num), 64)
	if err != nil {
		return 0
	}
	switch strings.ToLower(suffix) {
	case "k":
		f *= 1000
	case "m":
		f *= 1000000
	}
	return int64(f + 0.5)
}

// ExtractSessionID 从引擎 JSON 取会话 id（兼容 snake/camel 与 thread/chat）。
func ExtractSessionID(raw map[string]interface{}) string {
	if raw == nil {
		return ""
	}
	for _, k := range []string{"session_id", "sessionId", "thread_id", "threadId", "chat_id", "chatId"} {
		if s := jsonString(raw[k]); s != "" {
			return s
		}
	}
	if th, _ := raw["thread"].(map[string]interface{}); th != nil {
		if s := jsonString(th["id"]); s != "" {
			return s
		}
		if s := ExtractSessionID(th); s != "" {
			return s
		}
	}
	if msg, _ := raw["msg"].(map[string]interface{}); msg != nil {
		if s := ExtractSessionID(msg); s != "" {
			return s
		}
	}
	return ""
}

// StickySessionID 主会话只绑第一次出现的 id，避免子 agent/后到的 thread 覆盖导致下一轮失忆。
func StickySessionID(prev, incoming string) string {
	prev = strings.TrimSpace(prev)
	if prev != "" {
		return prev
	}
	return strings.TrimSpace(incoming)
}

func jsonString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	default:
		return ""
	}
}

func jsonInt64(v interface{}) int64 {
	switch t := v.(type) {
	case float64:
		return int64(t)
	case int64:
		return t
	case int:
		return int64(t)
	case json.Number:
		i, _ := t.Int64()
		return i
	case string:
		i, _ := strconv.ParseInt(t, 10, 64)
		return i
	default:
		return 0
	}
}
