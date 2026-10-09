package agent

import (
	"fmt"
	"strings"
	"time"
)

// ContextSnapshot Adapter 产出的统一上下文占用快照。
// 注意：Codex 的真实 active context occupancy 仅来自本机 rollout token_count；
// turn usage 与累计 session usage 都不能作为该值的替代。
type ContextSnapshot struct {
	SessionID    string
	UsedTokens   int64
	WindowTokens int64
	TotalTokens  int64
	UsedPercent  float64
	Estimated    bool
	Source       string // probe | codex_session | cached | unavailable
	ObservedAt   time.Time
	Anomaly      string
	TextExtra    string // 附加说明（探测原文/降级原因）
}

// ContextBuildInput 构建快照所需输入；探测逻辑由调用方注入，避免 Service 分引擎分支。
type ContextBuildInput struct {
	Engine         string
	SessionID      string
	CachedUsed     int64
	CachedWindow   int64
	FallbackWindow int64 // 来自统一配置，勿在业务里硬编码
	AllowProbe     bool  // false：忙碌/无 session 时跳过探测
	Probe          func() (used, window int64, raw string, err error)
}

// NormalizeRunMetaContext 将本轮官方 usage 规范为「单轮估算上下文」。
// 重要：绝不累计多轮 token；turn usage ≠ 当前真实 context occupancy。
func NormalizeRunMetaContext(engine string, m RunMeta, fallbackWindow int64) RunMeta {
	used := m.UsedTokens
	if used == 0 {
		used = m.InputTokens + m.CacheRead
	}
	if used == 0 && m.OutputTokens > 0 {
		used = m.InputTokens + m.OutputTokens
	}
	m.UsedTokens = used
	if m.ContextWindow <= 0 && fallbackWindow > 0 {
		m.ContextWindow = fallbackWindow
	}
	// 所有引擎的 stream/JSONL usage 都只是本轮回报；Claude 真值靠 /context probe。
	m.Estimated = true
	_ = engine // 预留：若某引擎日后 stream 自带 occupancy 可在此分支改 Estimated=false
	return m
}

// ContextFromProbe Claude /context 成功时的真实占用。
func ContextFromProbe(sessionID string, used, window int64) ContextSnapshot {
	s := ContextSnapshot{
		SessionID:    sessionID,
		UsedTokens:   used,
		WindowTokens: window,
		Estimated:    false,
		Source:       "probe",
	}
	s.UsedPercent = PercentOf(used, window)
	return s
}

// ContextFromCodexSession 将 Codex rollout 中的 token_count 转为精确上下文快照。
func ContextFromCodexSession(c CodexSessionContext) ContextSnapshot {
	s := ContextSnapshot{
		SessionID:    c.SessionID,
		UsedTokens:   c.UsedTokens,
		WindowTokens: c.WindowTokens,
		TotalTokens:  c.TotalTokens,
		Estimated:    false,
		Source:       "codex_session",
		ObservedAt:   c.ObservedAt,
		Anomaly:      c.Anomaly,
	}
	s.UsedPercent = PercentOf(s.UsedTokens, s.WindowTokens)
	return s
}

// EstimatedContextFromCache 用最近一次落库/缓存 usage 做估算快照（Codex/Cursor/Claude 降级）。
func EstimatedContextFromCache(sessionID string, used, window, fallbackWindow int64) ContextSnapshot {
	if window <= 0 {
		window = fallbackWindow
	}
	s := ContextSnapshot{
		SessionID:    sessionID,
		UsedTokens:   used,
		WindowTokens: window,
		Estimated:    true,
		Source:       "cached",
	}
	s.UsedPercent = PercentOf(used, window)
	return s
}

// BuildEngineContextSnapshot 按引擎规则产出统一快照（探测/估算/不可用）。
func BuildEngineContextSnapshot(in ContextBuildInput) ContextSnapshot {
	eng := strings.ToLower(strings.TrimSpace(in.Engine))
	if in.SessionID == "" && in.CachedUsed <= 0 {
		return ContextSnapshot{Source: "unavailable", Estimated: true}
	}

	switch eng {
	case "codex":
		// Codex 必须由调用方提供 rollout token_count；不能退回单轮 usage。
		return ContextSnapshot{SessionID: in.SessionID, Source: "unavailable", Estimated: true}
	case "claude":
		if in.AllowProbe && in.Probe != nil && strings.TrimSpace(in.SessionID) != "" {
			used, window, raw, err := in.Probe()
			if err == nil && window > 0 {
				s := ContextFromProbe(in.SessionID, used, window)
				s.TextExtra = strings.TrimSpace(raw)
				return s
			}
			// probe 失败 → 回退 stream/缓存估算
			s := EstimatedContextFromCache(in.SessionID, in.CachedUsed, in.CachedWindow, in.FallbackWindow)
			if err != nil {
				s.TextExtra = "（实时探测失败，已回退最近引擎回报：" + err.Error() + "）"
			}
			if raw != "" && s.TextExtra == "" {
				s.TextExtra = truncateRunes(raw, 800)
			}
			if s.UsedTokens <= 0 {
				s.Source = "unavailable"
				s.TextExtra = "Claude /context 探测失败"
				if err != nil {
					s.TextExtra += "：" + err.Error()
				}
				if raw != "" {
					s.TextExtra += "\n" + truncateRunes(raw, 800)
				}
			}
			return s
		}
		// 无探测：缓存估算
		if in.CachedUsed > 0 && (in.CachedWindow > 0 || in.FallbackWindow > 0) {
			return EstimatedContextFromCache(in.SessionID, in.CachedUsed, in.CachedWindow, in.FallbackWindow)
		}
		return ContextSnapshot{SessionID: in.SessionID, Source: "unavailable", Estimated: true}

	default:
		// Cursor Agent：无官方 active context API → 固定估算
		if in.CachedUsed > 0 || in.CachedWindow > 0 || in.FallbackWindow > 0 {
			s := EstimatedContextFromCache(in.SessionID, in.CachedUsed, in.CachedWindow, in.FallbackWindow)
			switch eng {
			case "agent":
				s.TextExtra = "（Cursor Agent：展示最近一轮 turn usage 估算。turn usage ≠ 真实 context occupancy）"
			}
			if s.UsedTokens <= 0 && s.WindowTokens <= 0 {
				s.Source = "unavailable"
			}
			return s
		}
		return ContextSnapshot{SessionID: in.SessionID, Source: "unavailable", Estimated: true}
	}
}

// InferEstimatedFromLegacy 旧数据无 estimated 字段时按 source/engine 推断。
func InferEstimated(engine, source string) bool {
	eng := strings.ToLower(strings.TrimSpace(engine))
	src := strings.ToLower(strings.TrimSpace(source))
	if (eng == "claude" && src == "probe") || (eng == "codex" && src == "codex_session") {
		return false
	}
	return true
}

// PercentOf used/window 百分比；window<=0 时返回 0。
func PercentOf(used, window int64) float64 {
	if window <= 0 {
		return 0
	}
	return float64(used) * 100 / float64(window)
}

// FormatContextPercent 展示用：估算带「约」。
func FormatContextPercent(pct float64, estimated bool) string {
	if estimated {
		return fmt.Sprintf("约 %.0f%%", pct)
	}
	return fmt.Sprintf("%.0f%%", pct)
}
