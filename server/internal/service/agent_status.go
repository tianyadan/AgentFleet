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

// EngineContext 引擎真实上下文占用（供状态按钮与圆环）。
type EngineContext struct {
	Engine       string `json:"engine"`
	SessionID    string `json:"session_id"`
	UsedTokens   int64  `json:"used_tokens"`
	WindowTokens int64  `json:"window_tokens"`
	Source       string `json:"source"` // probe | cached | unavailable
	Text         string `json:"text"`
	UpdatedAt    int64  `json:"updated_at"`
}

var (
	ctxCacheMu sync.Mutex
	ctxCache   = map[int64]EngineContext{} // agentID -> last
)

// FetchEngineContext 绑定引擎会话拉取真实上下文；fresh=true 时强制探测。
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
		out.Text = "尚无引擎会话：请先发送一轮对话后再查看上下文。"
		return out, nil
	}
	meta, _ := s.Store.GetConversationEngineMeta(ctx, a.ConversationID)
	out.SessionID = meta.SessionID
	out.UsedTokens = meta.UsedTokens
	out.WindowTokens = meta.WindowTokens

	if !fresh {
		ctxCacheMu.Lock()
		if c, ok := ctxCache[agentID]; ok && time.Now().Unix()-c.UpdatedAt < 4 && c.WindowTokens > 0 {
			ctxCacheMu.Unlock()
			return c, nil
		}
		ctxCacheMu.Unlock()
		if meta.UsedTokens > 0 && meta.WindowTokens > 0 {
			out.Source = "cached"
			out.Text = formatContextText(a, out)
			return out, nil
		}
	}

	bin := a.BinPath
	if bin == "" {
		bin = store.DefaultBin(a.Engine)
	}
	dir := s.Cfg.WorkspaceRoot
	if ws := strings.TrimSpace(a.WorkspacePath); ws != "" {
		dir = ws
	}

	switch strings.ToLower(a.Engine) {
	case "claude":
		if meta.SessionID == "" {
			out.Source = "unavailable"
			out.Text = "尚无 Claude session_id：完成一轮对话后会自动绑定。"
			return out, nil
		}
		used, window, raw, perr := agent.ClaudeProbeContext(ctx, bin, dir, meta.SessionID, 50*time.Second)
		if perr != nil {
			// 探测失败则回退缓存
			if meta.UsedTokens > 0 && meta.WindowTokens > 0 {
				out.Source = "cached"
				out.Text = formatContextText(a, out) + "\n（实时探测失败，已回退最近引擎回报：" + perr.Error() + "）"
				return out, nil
			}
			out.Source = "unavailable"
			out.Text = "Claude /context 探测失败：" + perr.Error() + "\n" + truncate(raw, 800)
			return out, nil
		}
		out.UsedTokens = used
		out.WindowTokens = window
		out.Source = "probe"
		_ = s.Store.UpdateConversationEngineMeta(ctx, a.ConversationID, meta.SessionID, used, window)
		out.Text = formatContextText(a, out) + "\n\n" + truncate(raw, 1200)
	case "agent":
		// Cursor：优先缓存；若有 session 可后续扩展 resume 探测
		if meta.UsedTokens > 0 && meta.WindowTokens > 0 {
			out.Source = "cached"
			out.Text = formatContextText(a, out) + "\n（Cursor Agent：展示最近一次 stream 回报的真实用量）"
		} else {
			out.Source = "unavailable"
			out.Text = "Cursor Agent 尚无用量数据：请先完成一轮对话。"
		}
	case "codex":
		if meta.UsedTokens > 0 && meta.WindowTokens > 0 {
			out.Source = "cached"
			out.Text = formatContextText(a, out) + "\n（Codex：展示最近一次可用用量；CLI 无统一 /context）"
		} else {
			out.Source = "unavailable"
			out.Text = "Codex 尚无用量数据：当前 CLI 未暴露稳定上下文探测接口。"
		}
	default:
		out.Source = "unavailable"
		out.Text = "未知引擎"
	}

	ctxCacheMu.Lock()
	ctxCache[agentID] = out
	ctxCacheMu.Unlock()
	return out, nil
}

func formatContextText(a *store.ManagedAgent, c EngineContext) string {
	pct := 0.0
	if c.WindowTokens > 0 {
		pct = float64(c.UsedTokens) * 100 / float64(c.WindowTokens)
	}
	return fmt.Sprintf(
		"状态快照 · %s\n引擎：%s\nsession：%s\n上下文占用：%s / %s（%.1f%%）\n来源：%s\n（不入库、不进模型上下文）",
		a.Name, a.Engine, fallback(c.SessionID, "-"),
		formatTokenCount(c.UsedTokens), formatTokenCount(c.WindowTokens), pct, c.Source,
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

// RememberEngineMeta 写入会话并刷新内存缓存。
func (s *Service) RememberEngineMeta(ctx context.Context, agentID, convID int64, meta agent.RunMeta) {
	if convID <= 0 {
		return
	}
	used := meta.UsedTokens
	if used == 0 {
		used = meta.InputTokens + meta.CacheRead
	}
	_ = s.Store.UpdateConversationEngineMeta(ctx, convID, meta.SessionID, used, meta.ContextWindow)
	if agentID > 0 && (used > 0 || meta.ContextWindow > 0) {
		ec := EngineContext{
			Engine: "", SessionID: meta.SessionID, UsedTokens: used, WindowTokens: meta.ContextWindow,
			Source: "cached", UpdatedAt: time.Now().Unix(),
		}
		ctxCacheMu.Lock()
		ctxCache[agentID] = ec
		ctxCacheMu.Unlock()
	}
}
