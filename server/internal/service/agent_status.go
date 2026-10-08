package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"colleague-avatar/server/internal/agent"
	"colleague-avatar/server/internal/store"
)

// EngineContext 引擎上下文占用（供状态按钮与圆环）。
// Estimated=false 仅表示 Claude /context 等真实探测；Codex/Cursor 恒为估算。
type EngineContext struct {
	Engine       string  `json:"engine"`
	SessionID    string  `json:"session_id"`
	UsedTokens   int64   `json:"used_tokens"`
	WindowTokens int64   `json:"window_tokens"`
	UsedPercent  float64 `json:"used_percent"`
	Estimated    bool    `json:"estimated"`
	Source       string  `json:"source"` // probe | cached | unavailable
	Text         string  `json:"text"`
	UpdatedAt    int64   `json:"updated_at"`
}

var (
	ctxCacheMu sync.Mutex
	ctxCache   = map[int64]EngineContext{} // agentID -> last
)

// FetchEngineContext 拉取统一 EngineContext；fresh=true 时 Claude 强制 /context 探测。
// 引擎差异在 agent.BuildEngineContextSnapshot 内处理，此处只做会话准备与落库。
func (s *Service) FetchEngineContext(ctx context.Context, agentID int64, fresh bool) (EngineContext, error) {
	a, err := s.Store.GetManagedAgent(ctx, agentID)
	if err != nil || a == nil {
		if a == nil && err == nil {
			return EngineContext{}, fmt.Errorf("not found")
		}
		return EngineContext{}, err
	}
	out := EngineContext{Engine: a.Engine, UpdatedAt: time.Now().Unix()}
	if a.ConversationID <= 0 {
		out.Source = "unavailable"
		out.Estimated = true
		out.Text = "尚无引擎会话：请先发送一轮对话后再查看上下文。"
		return out, nil
	}
	meta, _ := s.Store.GetConversationEngineMeta(ctx, a.ConversationID)
	out.SessionID = meta.SessionID
	out.UsedTokens = meta.UsedTokens
	out.WindowTokens = a.ContextWindowTokens

	if !fresh {
		ctxCacheMu.Lock()
		if c, ok := ctxCache[agentID]; ok && time.Now().Unix()-c.UpdatedAt < 4 && c.WindowTokens > 0 {
			c.WindowTokens = a.ContextWindowTokens
			c.UsedPercent = agent.PercentOf(c.UsedTokens, c.WindowTokens)
			if c.Source != "probe" {
				c.Estimated = agent.InferEstimated(a.Engine, c.Source)
			}
			ctxCacheMu.Unlock()
			return c, nil
		}
		ctxCacheMu.Unlock()
	}

	bin := a.BinPath
	if bin == "" {
		bin = store.DefaultBin(a.Engine)
	}
	dir := s.Cfg.WorkspaceRoot
	if ws := strings.TrimSpace(a.WorkspacePath); ws != "" {
		dir = ws
	}
	fallbackWin := a.ContextWindowTokens
	busy := contextProbeBlocked(a.Status, s.agentRunActive(agentID))
	allowProbe := fresh && !busy && strings.EqualFold(a.Engine, "claude")

	snap := agent.BuildEngineContextSnapshot(agent.ContextBuildInput{
		Engine:         a.Engine,
		SessionID:      meta.SessionID,
		CachedUsed:     meta.UsedTokens,
		CachedWindow:   meta.WindowTokens,
		FallbackWindow: fallbackWin,
		AllowProbe:     allowProbe,
		Probe: func() (used, window int64, raw string, err error) {
			return agent.ClaudeProbeContext(ctx, bin, dir, meta.SessionID, 50*time.Second)
		},
	})

	// 忙碌：禁止探测，统一回退缓存估算
	if busy {
		snap = agent.EstimatedContextFromCache(meta.SessionID, meta.UsedTokens, meta.WindowTokens, fallbackWin)
		snap.TextExtra = "对话进行中，已跳过 /context 探测以免打断当前回复。"
	}

	// 无数据时的引擎文案
	if strings.EqualFold(a.Engine, "claude") && meta.SessionID == "" && snap.Source == "unavailable" {
		snap.TextExtra = "尚无 Claude session_id：完成一轮对话后会自动绑定。"
	}
	if !strings.EqualFold(a.Engine, "claude") && snap.Source == "unavailable" {
		if strings.EqualFold(a.Engine, "codex") {
			snap.TextExtra = "Codex 尚无用量数据：当前 CLI 未暴露稳定上下文探测接口。"
		} else {
			snap.TextExtra = "Cursor Agent 尚无用量数据：请先完成一轮对话。"
		}
	}

	out = engineContextFromSnapshot(a.Engine, snap)
	out.UpdatedAt = time.Now().Unix()
	if out.Source != "probe" {
		out.Estimated = agent.InferEstimated(a.Engine, out.Source)
	}
	out.Text = formatContextText(a, out)
	if snap.TextExtra != "" {
		if out.Text != "" {
			out.Text += "\n" + snap.TextExtra
		} else {
			out.Text = snap.TextExtra
		}
	}

	if snap.Source == "probe" && snap.WindowTokens > 0 {
		_ = s.Store.UpdateConversationEngineMeta(ctx, a.ConversationID, meta.SessionID, snap.UsedTokens, snap.WindowTokens)
	}

	out.WindowTokens = a.ContextWindowTokens
	out.UsedPercent = agent.PercentOf(out.UsedTokens, out.WindowTokens)
	ctxCacheMu.Lock()
	ctxCache[agentID] = out
	ctxCacheMu.Unlock()
	return out, nil
}

func engineContextFromSnapshot(engine string, snap agent.ContextSnapshot) EngineContext {
	return EngineContext{
		Engine:       engine,
		SessionID:    snap.SessionID,
		UsedTokens:   snap.UsedTokens,
		WindowTokens: snap.WindowTokens,
		UsedPercent:  snap.UsedPercent,
		Estimated:    snap.Estimated,
		Source:       snap.Source,
	}
}

func formatContextText(a *store.ManagedAgent, c EngineContext) string {
	pct := c.UsedPercent
	if pct == 0 && c.WindowTokens > 0 {
		pct = agent.PercentOf(c.UsedTokens, c.WindowTokens)
	}
	pctLabel := agent.FormatContextPercent(pct, c.Estimated)
	estNote := ""
	if c.Estimated {
		estNote = "（估算：最近一轮 turn usage，≠ 真实 context occupancy）"
	}
	return fmt.Sprintf(
		"状态快照 · %s\n引擎：%s\nsession：%s\n上下文占用：%s / %s（%s）\n来源：%s%s\n（不入库、不进模型上下文）",
		a.Name, a.Engine, fallback(c.SessionID, "-"),
		formatTokenCount(c.UsedTokens), formatTokenCount(c.WindowTokens), pctLabel, c.Source, estNote,
	)
}

func formatTokenCount(n int64) string {
	if n >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

func fallback(s, d string) string {
	if strings.TrimSpace(s) == "" {
		return d
	}
	return s
}

// BuildAgentStatusReport 状态按钮文案（优先引擎真实占用）。
func (s *Service) BuildAgentStatusReport(ctx context.Context, agentID int64) (string, error) {
	c, err := s.FetchEngineContext(ctx, agentID, true)
	if err != nil {
		return "", err
	}
	return c.Text, nil
}

// agentRunActive 该数字员工是否有进行中的 Ask。
func (s *Service) agentRunActive(agentID int64) bool {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	_, ok := s.runs[agentID]
	return ok
}

// contextProbeBlocked 运行中禁止再开一个 claude --resume /context。
func contextProbeBlocked(status string, busy bool) bool {
	if busy {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "running", "waiting", "compressing", "initializing":
		return true
	default:
		return false
	}
}

// RememberEngineMeta 写入会话并刷新内存缓存。
// meta 经 Adapter 规范化：本轮 usage 作估算占用，不累计多轮。
func (s *Service) RememberEngineMeta(ctx context.Context, agentID, convID int64, meta agent.RunMeta) {
	if convID <= 0 {
		return
	}
	engine := ""
	if agentID > 0 {
		if a, _ := s.Store.GetManagedAgent(ctx, agentID); a != nil {
			engine = a.Engine
		}
	}
	if engine == "" {
		engine = "claude" // 首页对话默认 Claude Runner
	}
	// 主会话 id 只绑第一次：后到的子 agent / 新 thread 不得覆盖，否则下一轮 resume 到空会话。
	if prev, err := s.Store.GetConversationEngineMeta(ctx, convID); err == nil {
		meta.SessionID = agent.StickySessionID(prev.SessionID, meta.SessionID)
	}
	meta = agent.NormalizeRunMetaContext(engine, meta, s.Cfg.ContextWindowForEngine(engine))
	used := meta.UsedTokens
	_ = s.Store.UpdateConversationEngineMeta(ctx, convID, meta.SessionID, used, meta.ContextWindow)
	if agentID > 0 && (used > 0 || meta.ContextWindow > 0) {
		ec := EngineContext{
			Engine: engine, SessionID: meta.SessionID, UsedTokens: used, WindowTokens: meta.ContextWindow,
			UsedPercent: agent.PercentOf(used, meta.ContextWindow),
			Estimated:   true, // stream/JSONL 路径恒为估算；probe 成功会覆盖
			Source:      "cached", UpdatedAt: time.Now().Unix(),
		}
		ctxCacheMu.Lock()
		ctxCache[agentID] = ec
		ctxCacheMu.Unlock()
	}
}

// ResetEngineContextAfterCompact compact / session reset 后清零估算占用（保留 session 与窗口）。
func (s *Service) ResetEngineContextAfterCompact(ctx context.Context, agentID, convID int64) {
	if convID > 0 {
		_ = s.Store.ResetConversationUsedTokens(ctx, convID)
	}
	if agentID > 0 {
		ctxCacheMu.Lock()
		if c, ok := ctxCache[agentID]; ok {
			c.UsedTokens = 0
			c.UsedPercent = 0
			c.Estimated = true
			c.Source = "cached"
			c.UpdatedAt = time.Now().Unix()
			ctxCache[agentID] = c
		}
		ctxCacheMu.Unlock()
	}
}
