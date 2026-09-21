package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"colleague-avatar/server/internal/agent"
	"colleague-avatar/server/internal/store"
)

// AskEventSink SSE/进度事件回调。
type AskEventSink func(ev map[string]any)

// InvokeResult 单个被调同事的隔离结果。
type InvokeResult struct {
	AgentID   int64  `json:"agent_id"`
	AgentName string `json:"agent_name"`
	Engine    string `json:"engine"`
	Content   string `json:"content"`
	Err       string `json:"error,omitempty"`
	Ms        int    `json:"reply_ms"`
}

// emitAsk 安全发送事件。
func emitAsk(sink AskEventSink, ev map[string]any) {
	if sink != nil {
		sink(ev)
	}
}

// acquireAgentRun 若目标忙碌则等待；成功后持有 runs[agentID]，调用方须 releaseAgentRun(slot)。
func (s *Service) acquireAgentRun(ctx context.Context, agentID int64, onWaiting func()) (context.Context, *managedRun, error) {
	for {
		s.runMu.Lock()
		if _, busy := s.runs[agentID]; !busy {
			runCtx, cancel := context.WithCancel(ctx)
			slot := &managedRun{cancel: cancel}
			s.runs[agentID] = slot
			s.runMu.Unlock()
			return runCtx, slot, nil
		}
		s.runMu.Unlock()
		if onWaiting != nil {
			onWaiting()
		}
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-time.After(400 * time.Millisecond):
		}
	}
}

func (s *Service) releaseAgentRun(agentID int64, slot *managedRun) {
	if slot == nil {
		return
	}
	s.runMu.Lock()
	if s.runs[agentID] == slot {
		delete(s.runs, agentID)
	}
	s.runMu.Unlock()
	slot.cancel()
}

// runManagedEngine 跑引擎;permConvID/policyAgentID 控制授权路由(委托时 conv=调用方,policy=被调方)。
// noHook=true 用于任务拆分等轻量调用,避免再次弹授权。
func (s *Service) runManagedEngine(ctx context.Context, a *store.ManagedAgent, sys, question, history string, onChunk func(string), onActivity agent.ActivityFn, permConvID, policyAgentID int64, noHook bool, onEvent AskEventSink) (string, int, error) {
	bin := a.BinPath
	if bin == "" {
		bin = store.DefaultBin(a.Engine)
	}
	dir := s.Cfg.WorkspaceRoot
	if ws := strings.TrimSpace(a.WorkspacePath); ws != "" {
		dir = ws
	}
	timeout := s.Cfg.AskTimeout()
	metaConv := permConvID
	if metaConv == 0 {
		metaConv = a.ConversationID
	}
	// noHook：拆分/摘要等轻量调用，禁止污染主会话 engine_session（否则 Claude resume 会卡在「只输出 JSON」）
	persistSession := !noHook && metaConv > 0
	onMeta := func(m agent.RunMeta) {
		if persistSession {
			s.RememberEngineMeta(context.WithoutCancel(ctx), a.ID, metaConv, m)
		}
		turn := m.InputTokens + m.OutputTokens + m.CacheRead + m.CacheWrite
		emitAsk(onEvent, map[string]any{
			"type":               "usage",
			"session_id":         m.SessionID,
			"input_tokens":       m.InputTokens,
			"output_tokens":      m.OutputTokens,
			"cache_read_tokens":  m.CacheRead,
			"cache_write_tokens": m.CacheWrite,
			"used_tokens":        m.UsedTokens,
			"window_tokens":      m.ContextWindow,
			"turn_tokens":        turn,
		})
	}
	resume := ""
	if persistSession {
		if em, err := s.Store.GetConversationEngineMeta(ctx, metaConv); err == nil {
			resume = em.SessionID
		}
	}
	switch strings.ToLower(a.Engine) {
	case "claude":
		r := agent.NewRunner(bin)
		hook := agent.HookSpec{}
		// HookBin 存在即注入：PreToolUse + Pre/PostCompact（压缩同步不依赖 PermissionEnabled）
		if !noHook && s.Cfg.HookBin != "" {
			cid := permConvID
			if cid == 0 {
				cid = a.ConversationID
			}
			pid := policyAgentID
			if pid == 0 {
				pid = a.ID
			}
			hook = agent.HookSpec{
				Bin: s.Cfg.HookBin, Backend: s.Cfg.BackendURL,
				ConvID: cid, PolicyAgentID: pid,
				TimeoutS: s.Cfg.PermissionWaitSeconds() + 30,
			}
		}
		return r.AskStream(ctx, dir, sys, question, history, onChunk, timeout, hook, onActivity, onMeta, resume)
	case "codex":
		// 不写项目 .codex/hooks：Pre/PostCompact 会与 ask 竞态清库，导致「回复完过一会消失」。
		// 仅监听 JSONL compaction，并延后到 finishManagedAsk 之后再同步。
		onCmd := func(ev agent.CodexCommandEvent) {
			if !ev.Done {
				if onActivity != nil {
					onActivity(agent.Activity{Tool: "Bash", Summary: "执行命令 · " + truncateRunes(ev.Command, 72)})
				}
				return
			}
			summary := strings.TrimSpace(ev.Command)
			emitAsk(onEvent, map[string]any{
				"type": "command", "tool_name": "Bash", "summary": summary,
				"decision": "allow", "decided_by": "codex",
				"risk": "low", "meaning": "Codex 已执行命令",
				"output": ev.Output, "exit_code": ev.ExitCode,
			})
			if metaConv > 0 {
				s.recordCommand(context.WithoutCancel(ctx), metaConv, "Bash", summary, "allow", "codex", "low", "Codex 已执行命令", strings.TrimSpace(ev.Output))
			}
		}
		onCompact := func() {
			// 只打标，等回复落库后再 Sync，避免 loadDetail 读到被清空的库
			s.markCompactPending(metaConv)
		}
		return agent.CodexAskMetaEx(ctx, bin, dir, sys, question, history, onChunk, timeout, onActivity, onMeta, resume, onCmd, onCompact)
	default:
		// Cursor Agent 等：--resume + JSON 解析 session_id
		if !noHook && s.Cfg.HookBin != "" && metaConv > 0 {
			agent.EnsureCursorCompactHook(dir, s.Cfg.HookBin, s.Cfg.BackendURL, metaConv)
		}
		return agent.GenericAskMeta(ctx, bin, dir, sys, question, history, onChunk, timeout, onActivity, onMeta, resume)
	}
}

// invokeOne 委托单个同事(不写入其对话正文,只返回结果)。
func (s *Service) invokeOne(ctx context.Context, caller *store.ManagedAgent, m Mention, taskBody string, sink AskEventSink) InvokeResult {
	res := InvokeResult{AgentID: m.ID, AgentName: m.Name}
	emitAsk(sink, map[string]any{
		"type": "invoke_status", "callee_id": m.ID, "callee_name": m.Name, "status": "waiting",
	})
	var waitingOnce sync.Once
	runCtx, slot, err := s.acquireAgentRun(ctx, m.ID, func() {
		waitingOnce.Do(func() {
			_ = s.Store.SetManagedAgentStatus(context.WithoutCancel(ctx), m.ID, "waiting", "", 0, 0)
			emitAsk(sink, map[string]any{
				"type": "invoke_status", "callee_id": m.ID, "callee_name": m.Name, "status": "waiting",
			})
		})
	})
	if err != nil {
		res.Err = err.Error()
		emitAsk(sink, map[string]any{
			"type": "invoke_status", "callee_id": m.ID, "callee_name": m.Name, "status": "error", "message": res.Err,
		})
		return res
	}
	defer s.releaseAgentRun(m.ID, slot)

	a, err := s.Store.GetManagedAgent(runCtx, m.ID)
	if err != nil || a == nil {
		res.Err = "agent not found"
		emitAsk(sink, map[string]any{
			"type": "invoke_status", "callee_id": m.ID, "callee_name": m.Name, "status": "error", "message": res.Err,
		})
		return res
	}
	res.AgentName = a.Name
	res.Engine = a.Engine

	if a.ConversationID == 0 {
		cid, e := s.Store.CreateAgentConversation(runCtx, a.ID)
		if e == nil {
			_ = s.Store.SetManagedAgentConversation(runCtx, a.ID, cid)
			a.ConversationID = cid
		}
	}
	_ = s.Store.SetManagedAgentStatus(runCtx, a.ID, "running", "", 0, a.ConversationID)
	emitAsk(sink, map[string]any{
		"type": "invoke_status", "callee_id": a.ID, "callee_name": a.Name, "status": "running",
	})

	sys := BuildAgentSystemPrompt(a)
	q := fmt.Sprintf("【协作任务】由同事「%s」委托你执行。请直接完成下列任务并给出结果,不要反问调用方。\n\n%s",
		caller.Name, strings.TrimSpace(taskBody))
	// 授权弹窗挂到调用方会话;策略仍用被调方
	permConv := caller.ConversationID
	if permConv == 0 {
		permConv = a.ConversationID
	}
	out, ms, runErr := s.runEngineWithRecover(runCtx, a, permConv, sys, q, "", nil, sink, taskBody)
	res.Ms = ms
	if runErr != nil {
		res.Err = runErr.Error()
		if out == "" {
			out = "❌ " + res.Err
		}
		_ = s.Store.SetManagedAgentStatus(context.WithoutCancel(runCtx), a.ID, "error", res.Err, ms, a.ConversationID)
		emitAsk(sink, map[string]any{
			"type": "invoke_status", "callee_id": a.ID, "callee_name": a.Name, "status": "error", "message": res.Err,
		})
	} else {
		_ = s.Store.SetManagedAgentStatus(context.WithoutCancel(runCtx), a.ID, "idle", "", ms, a.ConversationID)
		emitAsk(sink, map[string]any{
			"type": "invoke_status", "callee_id": a.ID, "callee_name": a.Name, "status": "done",
		})
	}
	res.Content = strings.TrimSpace(out)
	emitAsk(sink, map[string]any{
		"type": "invoke_result", "callee_id": a.ID, "callee_name": a.Name, "engine": a.Engine,
		"content": res.Content, "error": res.Err, "reply_ms": ms,
	})
	return res
}

// invokeAllParallel 并行委托;busy 时排队。
func (s *Service) invokeAllParallel(ctx context.Context, caller *store.ManagedAgent, mentions []Mention, taskBody string, sink AskEventSink) []InvokeResult {
	out := make([]InvokeResult, len(mentions))
	var wg sync.WaitGroup
	for i, m := range mentions {
		wg.Add(1)
		go func(i int, m Mention) {
			defer wg.Done()
			out[i] = s.invokeOne(ctx, caller, m, taskBody, sink)
		}(i, m)
	}
	wg.Wait()
	return out
}

func formatInvokeQuote(r InvokeResult) string {
	var b strings.Builder
	b.WriteString("引用 · ")
	b.WriteString(r.AgentName)
	if r.Engine != "" {
		b.WriteString("（")
		b.WriteString(r.Engine)
		b.WriteString("）")
	}
	b.WriteString("\n\n")
	if r.Err != "" && r.Content == "" {
		b.WriteString("❌ ")
		b.WriteString(r.Err)
	} else {
		b.WriteString(r.Content)
	}
	return b.String()
}

func buildSummaryQuestion(userTask string, results []InvokeResult) string {
	var b strings.Builder
	b.WriteString("用户原任务：\n")
	b.WriteString(userTask)
	b.WriteString("\n\n你已并行协作其他同事，以下是各自隔离结果（勿混淆归属）。请分别汇总每位同事做了什么、结果如何，并给出总体结论（已完成 / 部分完成 / 失败原因）。用简洁中文。\n\n")
	for i, r := range results {
		b.WriteString(fmt.Sprintf("### [%d] 同事：%s (id=%d)\n", i+1, r.AgentName, r.AgentID))
		if r.Err != "" {
			b.WriteString("错误：")
			b.WriteString(r.Err)
			b.WriteString("\n")
		}
		b.WriteString(r.Content)
		b.WriteString("\n\n")
	}
	return b.String()
}

// writeInvokeDividersAsync 谁调用谁总结：异步写入被调方分割线(不进上下文)。
func (s *Service) writeInvokeDividersAsync(caller *store.ManagedAgent, userTask string, results []InvokeResult) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		for _, r := range results {
			summary := s.summarizeForCallee(ctx, caller, userTask, r)
			a, err := s.Store.GetManagedAgent(ctx, r.AgentID)
			if err != nil || a == nil || a.ConversationID == 0 {
				continue
			}
			meta, _ := json.Marshal(map[string]any{
				"caller_id": caller.ID, "caller_name": caller.Name,
				"callee_id": r.AgentID, "task": truncate(userTask, 200),
			})
			body := fmt.Sprintf("---\n📞 被「%s」调用\n任务：%s\n摘要：%s\n---",
				caller.Name, truncate(userTask, 120), summary)
			_, _ = s.Store.InsertMessage(ctx, &store.Message{
				ConversationID:     a.ConversationID,
				Role:               "invoke_divider",
				Content:            body,
				Status:             "ok",
				ExcludeFromContext: true,
				MetaJSON:           string(meta),
			})
		}
	}()
}

func (s *Service) summarizeForCallee(ctx context.Context, caller *store.ManagedAgent, userTask string, r InvokeResult) string {
	prompt := fmt.Sprintf("用一两句中文总结：同事「%s」委托「%s」完成了什么、结果如何。只输出摘要正文。\n任务：%s\n结果：%s\n错误：%s",
		caller.Name, r.AgentName, truncate(userTask, 300), truncate(r.Content, 1500), r.Err)
	out, _, err := s.runManagedEngine(ctx, caller, "你是摘要助手,只输出一两句中文摘要。", prompt, "", nil, nil, 0, 0, true, nil)
	out = strings.TrimSpace(out)
	if err != nil || out == "" {
		if r.Err != "" {
			return "执行失败：" + truncate(r.Err, 160)
		}
		return truncate(r.Content, 200)
	}
	return truncate(out, 300)
}
