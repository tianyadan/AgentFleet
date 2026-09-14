package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"colleague-avatar/server/internal/store"
)

const managedMaxTurns = 20

// AskManaged 管理台智能体一轮对话;onEvent 推送 SSE 事件(chunk/invoke_*/…)。可被 StopManaged 取消。
func (s *Service) AskManaged(ctx context.Context, agentID int64, question string, onEvent AskEventSink) (*store.Message, error) {
	a, err := s.Store.GetManagedAgent(ctx, agentID)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, fmt.Errorf("agent not found")
	}
	if occ, _ := s.Store.GetOccupancy(ctx, agentID); occ != nil && occ.SourceType != SourceDirect {
		return nil, &OccupancyBusyError{Occ: occ}
	}
	_ = s.Store.ClearOccupancy(ctx, agentID)
	if err := s.TryAcquireOccupancy(ctx, store.Occupancy{
		AgentID: agentID, SourceType: SourceDirect, SourceID: fmt.Sprintf("%d", agentID),
		SourceName: a.Name, TaskName: truncate(question, 120),
	}); err != nil {
		return nil, err
	}
	defer s.ReleaseOccupancy(context.WithoutCancel(ctx), agentID)

	return s.askManagedCore(ctx, a, 0, question, true, onEvent)
}

// AskManagedIsolated 在指定会话执行(工作流节点)；不改写智能体主会话；占用由调用方持有。
func (s *Service) AskManagedIsolated(ctx context.Context, agentID, convID int64, question string, onEvent AskEventSink) (*store.Message, error) {
	a, err := s.Store.GetManagedAgent(ctx, agentID)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, fmt.Errorf("agent not found")
	}
	if convID <= 0 {
		return nil, fmt.Errorf("conversation required")
	}
	return s.askManagedCore(ctx, a, convID, question, false, onEvent)
}

func (s *Service) askManagedCore(ctx context.Context, a *store.ManagedAgent, fixedConvID int64, question string, bindMainConv bool, onEvent AskEventSink) (*store.Message, error) {
	onChunk := func(chunk string) {
		emitAsk(onEvent, map[string]any{"type": "chunk", "content": chunk})
	}
	agentID := a.ID
	question = strings.TrimSpace(question)
	if question == "" {
		return nil, fmt.Errorf("empty question")
	}
	userQuestion := question
	// 注入长期记忆（仅进引擎 prompt，不写入用户气泡）
	engineQuestion := question
	if s.Memory != nil {
		if mems, err := s.Memory.Retrieve(ctx, agentID, question, 5); err == nil {
			if block := FormatMemoriesForPrompt(mems); block != "" {
				engineQuestion = block + "\n" + question
			}
		}
	}

	runCtx, cancel := context.WithCancel(ctx)
	slot := &managedRun{cancel: cancel}
	s.runMu.Lock()
	if old, ok := s.runs[agentID]; ok {
		old.cancel()
	}
	s.runs[agentID] = slot
	s.runMu.Unlock()
	defer func() {
		s.runMu.Lock()
		if s.runs[agentID] == slot {
			delete(s.runs, agentID)
		}
		s.runMu.Unlock()
		cancel()
	}()

	convID := fixedConvID
	if convID == 0 {
		convID = a.ConversationID
		if convID == 0 {
			var err error
			convID, err = s.Store.CreateAgentConversation(runCtx, agentID)
			if err != nil {
				return nil, err
			}
			_ = s.Store.SetManagedAgentConversation(runCtx, agentID, convID)
			a.ConversationID = convID
		}
	}

	_ = s.Store.SetManagedAgentStatus(runCtx, agentID, "running", "", 0, statusConvID(bindMainConv, convID))
	if a.AutoReview {
		s.Perms.SetAuto(convID, true)
	}

	all, _ := s.Store.ListManagedAgents(runCtx)
	refs := make([]AgentRef, 0, len(all))
	for _, x := range all {
		if x.ID == agentID {
			continue
		}
		refs = append(refs, AgentRef{ID: x.ID, Name: x.Name})
	}
	mentions, taskBody := ParseMentions(userQuestion, refs)
	// 禁止委托自己
	filtered := mentions[:0]
	for _, m := range mentions {
		if m.ID != agentID {
			filtered = append(filtered, m)
		}
	}
	mentions = filtered

	calleeRefs := make([]AgentRef, 0, len(mentions))
	for _, m := range mentions {
		calleeRefs = append(calleeRefs, AgentRef{ID: m.ID, Name: m.Name})
	}
	// 先规划具体步骤(禁止空话),再进入执行
	steps := s.PlanConcreteSteps(runCtx, a, taskBody, calleeRefs)
	tp := s.beginTaskProgress(runCtx, agentID, truncateRunTitle(taskBody), steps)
	emitAsk(onEvent, map[string]any{"type": "task_progress", "task_id": tp.taskID, "steps": steps, "progress": 0})

	_, _ = s.Store.InsertMessage(runCtx, &store.Message{
		ConversationID: convID, Role: "user", Content: userQuestion, Status: "ok",
	})

	tp.advance(runCtx, 0, steps[0])

	if len(mentions) > 0 {
		return s.askManagedWithInvokes(runCtx, a, convID, bindMainConv, userQuestion, taskBody, mentions, steps, tp, onEvent, onChunk)
	}

	history := s.buildManagedHistory(runCtx, convID)
	sys := BuildAgentSystemPrompt(a)
	userTurns, _ := s.Store.CountUserMessages(runCtx, convID)
	q := MaybeReinforcePolicy(a, userTurns, engineQuestion)
	mid := 1
	if len(steps) > 2 {
		mid = len(steps) / 2
	}
	if mid < len(steps) {
		tp.advance(runCtx, mid, steps[mid])
	}

	out, ms, runErr := s.runEngineWithRecover(runCtx, a, convID, sys, q, history, onChunk, onEvent, userQuestion)
	return s.finishManagedAsk(runCtx, agentID, convID, bindMainConv, out, ms, runErr, tp)
}

func statusConvID(bind bool, convID int64) int64 {
	if bind {
		return convID
	}
	return 0
}

func (s *Service) askManagedWithInvokes(
	ctx context.Context,
	caller *store.ManagedAgent,
	convID int64,
	bindMainConv bool,
	rawQuestion, taskBody string,
	mentions []Mention,
	steps []string,
	tp *TaskProgress,
	onEvent AskEventSink,
	onChunk func(string),
) (*store.Message, error) {
	// 步骤 0 已是理解需求；从 1 开始委托
	results := s.invokeAllParallel(ctx, caller, mentions, taskBody, onEvent)
	for i, r := range results {
		stepIdx := i + 1
		if stepIdx < len(steps) {
			tp.advance(ctx, stepIdx, steps[stepIdx])
		}
		meta, _ := json.Marshal(map[string]any{
			"callee_id": r.AgentID, "callee_name": r.AgentName, "engine": r.Engine, "error": r.Err,
		})
		quote := formatInvokeQuote(r)
		_, _ = s.Store.InsertMessage(ctx, &store.Message{
			ConversationID: convID,
			Role:           "invoke_quote",
			Content:        quote,
			Status:         "ok",
			MetaJSON:       string(meta),
		})
		emitAsk(onEvent, map[string]any{
			"type": "invoke_quote", "callee_id": r.AgentID, "callee_name": r.AgentName,
			"content": quote, "collapsed": true,
		})
	}

	sumStep := len(steps) - 1
	if sumStep < 0 {
		sumStep = 0
	}
	tp.advance(ctx, sumStep, steps[sumStep])

	sys := BuildAgentSystemPrompt(caller) + "\n你是编排方：根据各智能体的隔离结果分别汇总，并标明归属。"
	history := s.buildManagedHistory(ctx, convID)
	sumQ := buildSummaryQuestion(taskBody, results)
	userTurns, _ := s.Store.CountUserMessages(ctx, convID)
	sumQ = MaybeReinforcePolicy(caller, userTurns, sumQ)

	out, ms, runErr := s.runEngineWithRecover(ctx, caller, convID, sys, sumQ, history, onChunk, onEvent, taskBody)
	msg, err := s.finishManagedAsk(ctx, caller.ID, convID, bindMainConv, out, ms, runErr, tp)
	s.writeInvokeDividersAsync(caller, taskBody, results)
	return msg, err
}

func (s *Service) buildManagedHistory(ctx context.Context, convID int64) string {
	turns, err := s.Store.RecentTurns(ctx, convID, managedMaxTurns)
	if err != nil || len(turns) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("<历史对话上下文>\n")
	for _, t := range turns {
		sb.WriteString("user: " + t.User + "\n")
		if t.Assistant != "" {
			sb.WriteString("assistant: " + truncate(t.Assistant, 2000) + "\n")
		}
	}
	sb.WriteString("</历史对话上下文>\n")
	return sb.String()
}

func (s *Service) finishManagedAsk(ctx context.Context, agentID, convID int64, bindMainConv bool, out string, ms int, runErr error, tp *TaskProgress) (*store.Message, error) {
	if ctx.Err() != nil && (runErr != nil || out == "") {
		out = "⏹ 已终止本次对话"
		runErr = nil
		if ms == 0 {
			ms = 1
		}
	}
	status := "ok"
	lastErr := ""
	agentStatus := "idle"
	if runErr != nil {
		status = "error"
		lastErr = runErr.Error()
		agentStatus = "error"
		if out == "" {
			out = "❌ " + lastErr
		}
		if tp != nil {
			tp.fail(context.WithoutCancel(ctx), lastErr)
		}
	} else if tp != nil {
		tp.done(context.WithoutCancel(ctx), "已完成")
	}
	msg := &store.Message{
		ConversationID: convID, Role: "assistant", Content: out, Status: status, ReplyMs: ms,
	}
	_, _ = s.Store.InsertMessage(context.WithoutCancel(ctx), msg)
	_ = s.Store.SetManagedAgentStatus(context.WithoutCancel(ctx), agentID, agentStatus, lastErr, ms, statusConvID(bindMainConv, convID))
	return msg, nil
}

// StopManaged 终止该智能体当前进行中的 Ask。
func (s *Service) StopManaged(agentID int64) bool {
	s.runMu.Lock()
	slot, ok := s.runs[agentID]
	if ok {
		delete(s.runs, agentID)
	}
	s.runMu.Unlock()
	if !ok || slot == nil {
		return false
	}
	slot.cancel()
	a, _ := s.Store.GetManagedAgent(context.Background(), agentID)
	convID := int64(0)
	if a != nil {
		convID = a.ConversationID
	}
	_ = s.Store.SetManagedAgentStatus(context.Background(), agentID, "idle", "stopped by user", 0, convID)
	s.ReleaseOccupancy(context.Background(), agentID)
	return true
}

// ClearManagedChat 清空当前会话消息。
func (s *Service) ClearManagedChat(ctx context.Context, agentID int64) error {
	a, err := s.Store.GetManagedAgent(ctx, agentID)
	if err != nil || a == nil {
		if a == nil && err == nil {
			return fmt.Errorf("agent not found")
		}
		return err
	}
	if a.ConversationID == 0 {
		return nil
	}
	return s.Store.ClearConversationMessages(ctx, a.ConversationID)
}

// NewManagedChat 开启新对话(旧会话保留在 DB,但智能体指向新会话)。
func (s *Service) NewManagedChat(ctx context.Context, agentID int64) (int64, error) {
	a, err := s.Store.GetManagedAgent(ctx, agentID)
	if err != nil || a == nil {
		if a == nil && err == nil {
			return 0, fmt.Errorf("agent not found")
		}
		return 0, err
	}
	convID, err := s.Store.CreateAgentConversation(ctx, agentID)
	if err != nil {
		return 0, err
	}
	_ = s.Store.ClearConversationEngineMeta(ctx, convID)
	if err := s.Store.SetManagedAgentConversation(ctx, agentID, convID); err != nil {
		return 0, err
	}
	if a.AutoReview {
		s.Perms.SetAuto(convID, true)
	}
	return convID, nil
}

// CompressManagedChat 用 Claude 摘要压缩上下文,减少 token。
func (s *Service) CompressManagedChat(ctx context.Context, agentID int64) error {
	a, err := s.Store.GetManagedAgent(ctx, agentID)
	if err != nil || a == nil {
		if a == nil && err == nil {
			return fmt.Errorf("agent not found")
		}
		return err
	}
	if a.ConversationID == 0 {
		return nil
	}
	msgs, err := s.Store.Messages(ctx, a.ConversationID)
	if err != nil {
		return err
	}
	if len(msgs) == 0 {
		return nil
	}
	var sb strings.Builder
	for _, m := range msgs {
		if m.ExcludeFromContext {
			continue
		}
		if m.Role != "user" && m.Role != "assistant" {
			continue
		}
		sb.WriteString(m.Role + ": " + truncate(m.Content, 1500) + "\n")
	}
	prompt := "请把以下对话压缩成一段简洁的中文摘要,保留关键决策、文件路径与未完成事项,不要超过 800 字:\n\n" + sb.String()
	out, _, err := s.runManagedEngine(ctx, a, "你是对话摘要助手,只输出摘要正文。", prompt, "", nil, nil, 0, 0, true, nil)
	if err != nil {
		return err
	}
	summary := strings.TrimSpace(out)
	if summary == "" {
		return fmt.Errorf("empty summary")
	}
	if err := s.Store.ClearConversationMessages(ctx, a.ConversationID); err != nil {
		return err
	}
	_, err = s.Store.InsertMessage(ctx, &store.Message{
		ConversationID: a.ConversationID,
		Role:           "assistant",
		Content:        "📦 上下文已压缩:\n" + summary,
		Status:         "ok",
	})
	return err
}
