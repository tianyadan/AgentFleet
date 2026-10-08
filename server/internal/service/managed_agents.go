package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"colleague-avatar/server/internal/store"
)

// AskManaged 管理台数字员工一轮对话;onEvent 推送 SSE 事件(chunk/invoke_*/…)。可被 StopManaged 取消。
func (s *Service) AskManaged(ctx context.Context, agentID int64, question string, onEvent AskEventSink) (*store.Message, error) {
	a, err := s.Store.GetManagedAgent(ctx, agentID)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, fmt.Errorf("agent not found")
	}
	if err := GuardAgentUsable(a); err != nil {
		return nil, err
	}
	occ, _ := s.Store.GetOccupancy(ctx, agentID)
	if OccupancyBlocksUserChat(occ) {
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

	return s.askManagedCore(ctx, a, 0, question, askCoreOpts{bindMainConv: true}, onEvent)
}

// AskManagedIsolated 在指定会话执行(项目协作节点)；不改写数字员工主会话；占用由调用方持有。
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
	if err := GuardAgentUsable(a); err != nil {
		return nil, err
	}
	return s.askManagedCore(ctx, a, convID, question, askCoreOpts{bindMainConv: false}, onEvent)
}

type askCoreOpts struct {
	bindMainConv bool
	silent       bool
	holdStatus   bool
	skipTaskPlan bool
	// visitorAudit 非空时：用户消息落库后建 processing 审计，结束后写 token/耗时/状态。
	visitorAudit *visitorAskAudit
}

// visitorAskAudit 前台一轮 Ask 的审计上下文。
type visitorAskAudit struct {
	VisitorID int64
	IP        string
	UA        string
	logID     int64
	started   time.Time
	input     int64
	output    int64
	cacheRead int64
	cacheWrite int64
}

func (s *Service) silentAsk(ctx context.Context, a *store.ManagedAgent, question string, holdStatus bool) (*store.Message, error) {
	msg, err := s.askManagedCore(ctx, a, 0, question, askCoreOpts{
		bindMainConv: true,
		silent:       true,
		holdStatus:   holdStatus,
		skipTaskPlan: true,
	}, nil)
	if err != nil {
		return msg, err
	}
	if msg != nil && msg.Status == "error" {
		errText := strings.TrimSpace(strings.TrimPrefix(msg.Content, "❌ "))
		if errText == "" {
			errText = "silent ask failed"
		}
		return msg, fmt.Errorf("%s", errText)
	}
	return msg, nil
}

func (s *Service) askManagedCore(ctx context.Context, a *store.ManagedAgent, fixedConvID int64, question string, opts askCoreOpts, onEvent AskEventSink) (*store.Message, error) {
	// 先包装 onEvent，再定义 onChunk，确保 usage 能写入审计。
	if opts.visitorAudit != nil {
		inner := onEvent
		onEvent = func(ev map[string]any) {
			if ev != nil {
				if t, _ := ev["type"].(string); t == "usage" {
					opts.visitorAudit.input = toInt64Any(ev["input_tokens"])
					opts.visitorAudit.output = toInt64Any(ev["output_tokens"])
					opts.visitorAudit.cacheRead = toInt64Any(ev["cache_read_tokens"])
					opts.visitorAudit.cacheWrite = toInt64Any(ev["cache_write_tokens"])
				}
			}
			if inner != nil {
				inner(ev)
			}
		}
	}
	onChunk := func(chunk string) {
		emitAsk(onEvent, map[string]any{"type": "chunk", "content": chunk})
	}
	agentID := a.ID
	question = strings.TrimSpace(question)
	if question == "" {
		return nil, fmt.Errorf("empty question")
	}
	userQuestion := question
	// v0.2.18：不再注入平台历史/Memory；上下文靠引擎 resume + 本轮问题。
	engineQuestion := question

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

	if !opts.holdStatus {
		_ = s.Store.SetManagedAgentStatus(runCtx, agentID, "running", "", 0, statusConvID(opts.bindMainConv, convID))
	}
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
	if opts.silent {
		mentions = nil
		taskBody = userQuestion
	}
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

	if !opts.silent {
		// 先落库用户消息，再规划/执行，避免规划耗时或中途压缩清库导致对话框「发出去看不见」
		userMsg := &store.Message{
			ConversationID: convID, Role: "user", Content: userQuestion, Status: "ok",
		}
		userMsgID, uerr := s.Store.InsertMessage(runCtx, userMsg)
		if opts.visitorAudit != nil {
			opts.visitorAudit.started = time.Now()
			if uerr != nil {
				log.Printf("visitor ask: insert user message failed: %v", uerr)
			} else {
				logID, aerr := s.Store.BeginVisitorAskLog(runCtx, opts.visitorAudit.VisitorID, convID, agentID, userMsgID, opts.visitorAudit.IP, opts.visitorAudit.UA)
				if aerr != nil {
					log.Printf("visitor ask: begin audit failed: %v", aerr)
				} else {
					opts.visitorAudit.logID = logID
				}
			}
		}
	}

	var steps []string
	var tp *TaskProgress
	if shouldEmitTaskPlanAsk(a, opts.skipTaskPlan) {
		// 先规划具体步骤(禁止空话),再进入执行
		steps = s.PlanConcreteSteps(runCtx, a, taskBody, calleeRefs)
		tp = s.beginTaskProgress(runCtx, agentID, truncateRunTitle(taskBody), steps)
		emitAsk(onEvent, map[string]any{"type": "task_progress", "task_id": tp.taskID, "steps": steps, "progress": 0})
	}

	if tp != nil && len(steps) > 0 {
		tp.advance(runCtx, 0, steps[0])
	}

	if len(mentions) > 0 {
		return s.askManagedWithInvokes(runCtx, a, convID, opts, userQuestion, taskBody, mentions, steps, tp, onEvent, onChunk)
	}

	sys := BuildAgentSystemPrompt(a)
	sys = s.consumeSystemReinject(runCtx, convID, sys)
	userTurns, _ := s.Store.CountUserMessages(runCtx, convID)
	q := MaybeReinforcePolicy(a, userTurns, engineQuestion)
	mid := 1
	if len(steps) > 2 {
		mid = len(steps) / 2
	}
	if tp != nil && mid < len(steps) {
		tp.advance(runCtx, mid, steps[mid])
	}

	// history 有 session 时恒空；丢失时由 runManagedEngine 回退 RecentTurns
	out, ms, runErr := s.runEngineWithRecover(runCtx, a, convID, sys, q, "", onChunk, onEvent, userQuestion)
	msg, err := s.finishManagedAsk(runCtx, agentID, convID, opts.bindMainConv, out, ms, runErr, tp, opts)
	s.flushCompactAfterAsk(runCtx, convID, onEvent)
	s.finalizeVisitorAskAudit(runCtx, opts.visitorAudit, msg, runErr)
	return msg, err
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
	opts askCoreOpts,
	rawQuestion, taskBody string,
	mentions []Mention,
	steps []string,
	tp *TaskProgress,
	onEvent AskEventSink,
	onChunk func(string),
) (*store.Message, error) {
	bindMainConv := opts.bindMainConv
	// 步骤 0 已是理解需求；从 1 开始委托
	results := s.invokeAllParallel(ctx, caller, mentions, taskBody, onEvent)
	for i, r := range results {
		stepIdx := i + 1
		if tp != nil && stepIdx < len(steps) {
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

	if tp != nil && len(steps) > 0 {
		sumStep := len(steps) - 1
		if sumStep < 0 {
			sumStep = 0
		}
		tp.advance(ctx, sumStep, steps[sumStep])
	}
	sys := BuildAgentSystemPrompt(caller) + "\n你是协作编排方：根据各同事的隔离结果分别汇总，并标明归属。"
	sys = s.consumeSystemReinject(ctx, convID, sys)
	sumQ := buildSummaryQuestion(taskBody, results)
	userTurns, _ := s.Store.CountUserMessages(ctx, convID)
	sumQ = MaybeReinforcePolicy(caller, userTurns, sumQ)

	out, ms, runErr := s.runEngineWithRecover(ctx, caller, convID, sys, sumQ, "", onChunk, onEvent, taskBody)
	msg, err := s.finishManagedAsk(ctx, caller.ID, convID, bindMainConv, out, ms, runErr, tp, opts)
	s.flushCompactAfterAsk(ctx, convID, onEvent)
	s.finalizeVisitorAskAudit(ctx, opts.visitorAudit, msg, runErr)
	s.writeInvokeDividersAsync(caller, taskBody, results)
	return msg, err
}

// flushCompactAfterAsk 引擎回合内发生压缩时，在 assistant 落库后再同步平台历史。
func (s *Service) flushCompactAfterAsk(ctx context.Context, convID int64, onEvent AskEventSink) {
	if !s.consumeCompactPending(convID) {
		return
	}
	_ = s.SyncPlatformAfterCompact(context.WithoutCancel(ctx), convID)
	emitAsk(onEvent, map[string]any{
		"type": "context_compacted", "content": CompactNoticeText,
		"used_tokens": 0, "estimated": true, // compact 后重置估算占用
	})
}

func (s *Service) finishManagedAsk(ctx context.Context, agentID, convID int64, bindMainConv bool, out string, ms int, runErr error, tp *TaskProgress, opts askCoreOpts) (*store.Message, error) {
	out, runErr = applyCancelledAskOutput(ctx.Err(), out, runErr)
	if ctx.Err() != nil && ms == 0 {
		ms = 1
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
	if !opts.silent {
		if _, err := s.Store.InsertMessage(context.WithoutCancel(ctx), msg); err != nil {
			log.Printf("finishManagedAsk: insert assistant failed: %v", err)
		}
	}
	if !opts.holdStatus {
		_ = s.Store.SetManagedAgentStatus(context.WithoutCancel(ctx), agentID, agentStatus, lastErr, ms, statusConvID(bindMainConv, convID))
	}
	return msg, nil
}

// finalizeVisitorAskAudit 写完 assistant 后结束审计；失败只打日志，不拖垮主链路。
func (s *Service) finalizeVisitorAskAudit(ctx context.Context, audit *visitorAskAudit, msg *store.Message, runErr error) {
	if audit == nil || audit.logID <= 0 {
		return
	}
	status := store.AskStatusSuccess
	errMsg := ""
	if ctx.Err() != nil {
		status = store.AskStatusCancelled
		errMsg = ctx.Err().Error()
	} else if runErr != nil {
		status = store.AskStatusError
		errMsg = runErr.Error()
	}
	asstID := int64(0)
	duration := int(time.Since(audit.started).Milliseconds())
	if msg != nil {
		asstID = msg.ID
		if msg.Status == "error" && status == store.AskStatusSuccess {
			status = store.AskStatusError
		}
	}
	if duration <= 0 {
		duration = 1
	}
	cached := audit.cacheRead + audit.cacheWrite
	total := audit.input + audit.output + cached
	if err := s.Store.FinishVisitorAskLog(context.WithoutCancel(ctx), audit.logID, asstID,
		audit.input, audit.output, cached, total, duration, status, errMsg); err != nil {
		log.Printf("visitor ask: finish audit %d failed: %v", audit.logID, err)
	}
}

func toInt64Any(v any) int64 {
	switch t := v.(type) {
	case int64:
		return t
	case int:
		return int64(t)
	case float64:
		return int64(t)
	case json.Number:
		i, _ := t.Int64()
		return i
	default:
		return 0
	}
}

// applyCancelledAskOutput 请求被取消时保留已生成正文，避免刷新页面把回复覆盖成只有「已终止」。
// 「No response requested.」是 CLI 本地命令空转，不当成有效回复。
func applyCancelledAskOutput(ctxErr error, out string, runErr error) (string, error) {
	if ctxErr == nil {
		return out, runErr
	}
	trimmed := strings.TrimSpace(out)
	if trimmed == "" || trimmed == "No response requested." {
		return "⏹ 已终止本次对话", nil
	}
	if !strings.Contains(trimmed, "已终止本次对话") {
		trimmed += "\n\n⏹ 已终止本次对话"
	}
	return trimmed, nil
}

// StopManaged 终止该数字员工当前进行中的 Ask。
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
