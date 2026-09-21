package service

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"colleague-avatar/server/internal/store"
)

// WorkflowEngine 进程内 DAG 推进器。
type WorkflowEngine struct {
	svc *Service

	mu   sync.Mutex
	runs map[int64]context.CancelFunc // runID -> cancel
}

func newWorkflowEngine(svc *Service) *WorkflowEngine {
	return &WorkflowEngine{svc: svc, runs: map[int64]context.CancelFunc{}}
}

// StartWorkflowRun 创建并异步执行。forceTakeover 为 true 时先打断冲突员工再启动。
func (s *Service) StartWorkflowRun(ctx context.Context, defID int64, prompt string, forceTakeover bool) (*store.WorkflowRun, error) {
	def, err := s.Store.GetWorkflowDefinition(ctx, defID)
	if err != nil || def == nil {
		return nil, fmt.Errorf("workflow not found")
	}
	g, err := ParseWorkflowGraph(def.GraphJSON)
	if err != nil {
		return nil, fmt.Errorf("invalid graph: %w", err)
	}
	agentIDs := CollectAgentIDsFromGraph(g)
	if len(agentIDs) > 0 {
		occMap, _ := s.Store.ListOccupancyMap(ctx)
		names := map[int64]string{}
		for id := range agentIDs {
			if a, _ := s.Store.GetManagedAgent(ctx, id); a != nil {
				names[id] = a.Name
			}
		}
		conflicts := FilterStartConflicts(agentIDs, occMap, names)
		if len(conflicts) > 0 {
			if !forceTakeover {
				return nil, &WorkflowStartConflictError{Conflicts: conflicts}
			}
			if err := s.forceTakeoverAgents(ctx, conflicts); err != nil {
				return nil, err
			}
		}
	}
	runID, err := s.Store.CreateWorkflowRun(ctx, defID, prompt)
	if err != nil {
		return nil, err
	}
	_ = s.Store.UpdateWorkflowRunStatus(ctx, runID, "running", "", "", `[]`, `{}`, `{}`)
	run, _ := s.Store.GetWorkflowRun(ctx, runID)

	runCtx, cancel := context.WithCancel(context.Background())
	s.WF.mu.Lock()
	s.WF.runs[runID] = cancel
	s.WF.mu.Unlock()

	go func() {
		defer func() {
			s.WF.mu.Lock()
			delete(s.WF.runs, runID)
			s.WF.mu.Unlock()
			cancel()
		}()
		if err := s.WF.execute(runCtx, runID); err != nil {
			log.Printf("workflow run %d: %v", runID, err)
		}
	}()
	return run, nil
}

// forceTakeoverAgents 停止冲突员工 Ask / 取消复制总结，并释放占用。
func (s *Service) forceTakeoverAgents(ctx context.Context, conflicts []AgentConflict) error {
	for _, c := range conflicts {
		s.cancelCloneJob(c.AgentID)
		s.StopManaged(c.AgentID)
		_ = s.Store.ClearOccupancy(ctx, c.AgentID)
		// 若有人以该员工为源正在 initializing，标记错误（可选扫描）
		if c.Occupancy != nil && c.Occupancy.SourceType == SourceCloneInit {
			// 源被打断：查找 cloned_from_id=c.AgentID 且 initializing 的副本
			_ = s.markClonesInterruptedBySource(ctx, c.AgentID)
		}
	}
	return nil
}

func (s *Service) markClonesInterruptedBySource(ctx context.Context, srcID int64) error {
	items, err := s.Store.ListManagedAgents(ctx)
	if err != nil {
		return err
	}
	for _, a := range items {
		if a.ClonedFromID == srcID && a.Status == AgentStatusInitializing {
			_ = s.Store.SetManagedAgentStatus(ctx, a.ID, "error", "源被协作接管，总结中断", 0, a.ConversationID)
			s.cancelCloneJob(a.ID)
		}
	}
	return nil
}

// StopWorkflowRun 用户终止：取消工作流、杀掉占用中的数字员工 CLI 进程、释放占用。
func (s *Service) StopWorkflowRun(runID int64) error {
	s.WF.mu.Lock()
	cancel, ok := s.WF.runs[runID]
	s.WF.mu.Unlock()
	if ok && cancel != nil {
		cancel()
	}
	ctx := context.Background()
	s.stopWorkflowAgents(ctx, runID)
	run, err := s.Store.GetWorkflowRun(ctx, runID)
	if err != nil || run == nil {
		return fmt.Errorf("run not found")
	}
	if run.Status == "success" || run.Status == "failed" || run.Status == "stopped" || run.Status == "interrupted" {
		return nil
	}
	// waiting_recovery 也允许用户终止
	return s.Store.UpdateWorkflowRunStatus(ctx, runID, "stopped", run.FailNodeID, "用户终止", run.CurrentNodeIDsJSON, run.ProgressJSON, run.ParallelStateJSON)
}

// stopWorkflowAgents 终止该 run 仍在工作的数字员工进程并标记节点停止。
func (s *Service) stopWorkflowAgents(ctx context.Context, runID int64) {
	occ, _ := s.Store.ListOccupancyMap(ctx)
	nes, _ := s.Store.ListNodeExecutions(ctx, runID)
	for _, agentID := range CollectWorkflowStopAgentIDs(runID, occ, nes) {
		s.StopManaged(agentID)
	}
	for _, ne := range nes {
		if ne.ConversationID > 0 {
			s.Perms.DropByConv(ne.ConversationID)
		}
		if ne.Status == "running" || (ne.Status == "waiting" && ne.NodeType == "agent") {
			_ = s.Store.UpdateNodeExecution(ctx, ne.ID, "stopped", `{"status":"FAIL"}`, "用户终止工作流", ne.ConversationID)
		}
	}
}

func (e *WorkflowEngine) execute(ctx context.Context, runID int64) error {
	s := e.svc
	run, err := s.Store.GetWorkflowRun(ctx, runID)
	if err != nil || run == nil {
		return fmt.Errorf("run missing")
	}
	def, err := s.Store.GetWorkflowDefinition(ctx, run.DefinitionID)
	if err != nil || def == nil {
		return e.fail(ctx, runID, "", "definition missing")
	}
	g, err := ParseWorkflowGraph(def.GraphJSON)
	if err != nil {
		return e.fail(ctx, runID, "", err.Error())
	}
	start, err := g.StartNode()
	if err != nil {
		return e.fail(ctx, runID, "", err.Error())
	}

	outputs := map[string]string{} // nodeID -> latest output JSON
	prompt := run.InputPrompt

	// Start 节点
	if err := e.runStart(ctx, runID, start, prompt, outputs); err != nil {
		return e.fail(ctx, runID, start.ID, err.Error())
	}

	queue := g.Successors(start.ID)
	visitedOK := map[string]bool{start.ID: true}
	return e.runQueueWaves(ctx, runID, run.DefinitionID, def.Name, g, prompt, outputs, visitedOK, queue)
}

func (e *WorkflowEngine) executeFrom(ctx context.Context, runID int64, startNodeID string) error {
	return e.executeFromWithSeed(ctx, runID, []string{startNodeID}, nil)
}

func (e *WorkflowEngine) executeFromWithSeed(ctx context.Context, runID int64, starts []string, seedOutputs map[string]string) error {
	s := e.svc
	run, err := s.Store.GetWorkflowRun(ctx, runID)
	if err != nil || run == nil {
		return err
	}
	def, _ := s.Store.GetWorkflowDefinition(ctx, run.DefinitionID)
	g, err := ParseWorkflowGraph(def.GraphJSON)
	if err != nil {
		return e.fail(ctx, runID, "", err.Error())
	}
	outputs := map[string]string{}
	if seedOutputs != nil {
		for k, v := range seedOutputs {
			outputs[k] = v
		}
	} else {
		list, _ := s.Store.ListNodeExecutions(ctx, runID)
		latest := map[string]store.NodeExecution{}
		for _, ne := range list {
			prev, ok := latest[ne.NodeID]
			if !ok || ne.Attempt >= prev.Attempt {
				latest[ne.NodeID] = ne
			}
		}
		for id, ne := range latest {
			if ne.Status == "success" && ne.OutputJSON != "" {
				outputs[id] = ne.OutputJSON
			}
		}
	}
	visitedOK := map[string]bool{}
	for id, raw := range outputs {
		if raw != "" {
			visitedOK[id] = true
		}
	}
	queue := append([]string{}, starts...)
	return e.runQueueWaves(ctx, runID, run.DefinitionID, def.Name, g, run.InputPrompt, outputs, visitedOK, queue)
}

type waveNodeResult struct {
	node    GraphNode
	next    []string
	err     error
	waiting bool
}

// runQueueWaves 波次推进：同一波内就绪节点并发执行（真正并行）。
func (e *WorkflowEngine) runQueueWaves(
	ctx context.Context, runID, defID int64, defName string, g *WorkflowGraph,
	prompt string, outputs map[string]string, visitedOK map[string]bool, queue []string,
) error {
	s := e.svc
	var outMu sync.Mutex

	for len(queue) > 0 {
		if ctx.Err() != nil {
			_ = s.Store.UpdateWorkflowRunStatus(ctx, runID, "stopped", "", "cancelled", store.MustJSON(keys(visitedOK)), `{}`, `{}`)
			return ctx.Err()
		}

		var ready []GraphNode
		var deferred []string
		seenReady := map[string]bool{}
		for _, nodeID := range dedupeStrings(queue) {
			n, ok := g.Node(nodeID)
			if !ok {
				continue
			}
			if n.Type == "merge" {
				readyMerge, err := e.mergeReady(ctx, runID, g, n, visitedOK)
				if err != nil {
					return e.fail(ctx, runID, n.ID, err.Error())
				}
				if !readyMerge {
					deferred = append(deferred, nodeID)
					continue
				}
			}
			if seenReady[n.ID] {
				continue
			}
			seenReady[n.ID] = true
			ready = append(ready, n)
		}
		queue = deferred

		if len(ready) == 0 {
			if len(queue) > 0 {
				return e.fail(ctx, runID, queue[0], "merge 等待的前置分支未全部成功")
			}
			break
		}

		ids := make([]string, 0, len(ready))
		for _, n := range ready {
			ids = append(ids, n.ID)
		}
		_ = s.Store.UpdateWorkflowRunStatus(ctx, runID, "running", "", "", store.MustJSON(ids), store.MustJSON(map[string]any{"done": keys(visitedOK)}), `{}`)

		results := make([]waveNodeResult, len(ready))
		var wg sync.WaitGroup
		for i, n := range ready {
			wg.Add(1)
			go func(i int, n GraphNode) {
				defer wg.Done()
				outMu.Lock()
				localOut := copyStringMap(outputs)
				outMu.Unlock()

				next, err := e.runNode(ctx, runID, defID, defName, g, n, prompt, localOut)
				res := waveNodeResult{node: n, next: next, err: err}
				if err == nil && len(next) == 1 && next[0] == "waiting" {
					res.waiting = true
				}

				outMu.Lock()
				if v, ok := localOut[n.ID]; ok && v != "" {
					outputs[n.ID] = v
				}
				outMu.Unlock()
				results[i] = res
			}(i, n)
		}
		wg.Wait()

		for _, r := range results {
			if r.err != nil {
				return e.fail(ctx, runID, r.node.ID, r.err.Error())
			}
			if r.waiting {
				_ = s.Store.UpdateWorkflowRunStatus(ctx, runID, "waiting", "", "", store.MustJSON([]string{r.node.ID}), store.MustJSON(map[string]any{"done": keys(visitedOK)}), `{}`)
				return nil
			}
			visitedOK[r.node.ID] = true
			queue = appendUniqueQueue(queue, visitedOK, r.next...)
			if r.node.Type == "end" {
				_ = s.Store.UpdateWorkflowRunStatus(ctx, runID, "success", "", "", `[]`, store.MustJSON(map[string]any{"done": keys(visitedOK)}), `{}`)
				return nil
			}
		}
	}
	_ = s.Store.UpdateWorkflowRunStatus(ctx, runID, "success", "", "", `[]`, store.MustJSON(map[string]any{"done": keys(visitedOK)}), `{}`)
	return nil
}

func copyStringMap(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// dedupeStrings 保序去重。
func dedupeStrings(ids []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

// appendUniqueQueue 将后继入队：跳过已成功节点、已在队列中的节点。
func appendUniqueQueue(queue []string, visitedOK map[string]bool, ids ...string) []string {
	inQ := map[string]bool{}
	for _, id := range queue {
		inQ[id] = true
	}
	for _, id := range ids {
		if id == "" || visitedOK[id] || inQ[id] {
			continue
		}
		inQ[id] = true
		queue = append(queue, id)
	}
	return queue
}

func keys(m map[string]bool) []string {
	var out []string
	for k, v := range m {
		if v {
			out = append(out, k)
		}
	}
	return out
}

func (e *WorkflowEngine) fail(ctx context.Context, runID int64, nodeID, reason string) error {
	_ = e.svc.Store.UpdateWorkflowRunStatus(ctx, runID, "failed", nodeID, reason, store.MustJSON([]string{nodeID}), `{}`, `{}`)
	return fmt.Errorf("%s", reason)
}

func (e *WorkflowEngine) runStart(ctx context.Context, runID int64, n GraphNode, prompt string, outputs map[string]string) error {
	attempt, _ := e.svc.Store.NextNodeAttempt(ctx, runID, n.ID)
	ne := &store.NodeExecution{
		RunID: runID, NodeID: n.ID, Attempt: attempt, NodeType: "start", Status: "running",
		InputJSON: store.MustJSON(map[string]any{"prompt": prompt}),
	}
	id, err := e.svc.Store.InsertNodeExecution(ctx, ne)
	if err != nil {
		return err
	}
	out := store.MustJSON(map[string]any{"prompt": prompt, "text": prompt, "status": "PASS"})
	outputs[n.ID] = out
	return e.svc.Store.UpdateNodeExecution(ctx, id, "success", out, "", 0)
}

func (e *WorkflowEngine) mergeReady(ctx context.Context, runID int64, g *WorkflowGraph, n GraphNode, visited map[string]bool) (bool, error) {
	need := g.DataStringSlice(n, "wait_for_node_ids")
	if len(need) == 0 {
		// 默认所有入边
		for _, e := range g.ins[n.ID] {
			need = append(need, e.Source)
		}
	}
	for _, id := range need {
		latest, err := e.svc.Store.LatestNodeExecution(ctx, runID, id)
		if err != nil {
			return false, err
		}
		if latest == nil || latest.Status != "success" {
			return false, nil
		}
		visited[id] = true
	}
	return true, nil
}

// runNode 执行节点；返回后续 nodeIDs，或 "waiting"。
func (e *WorkflowEngine) runNode(ctx context.Context, runID, defID int64, defName string, g *WorkflowGraph, n GraphNode, prompt string, outputs map[string]string) ([]string, error) {
	switch n.Type {
	case "agent":
		return e.runAgent(ctx, runID, defID, defName, g, n, prompt, outputs)
	case "human_review", "review":
		return e.runHumanReview(ctx, runID, n)
	case "condition":
		return e.runCondition(ctx, runID, g, n, outputs)
	case "parallel":
		return e.runParallel(ctx, runID, g, n, outputs)
	case "merge":
		return e.runMergePass(ctx, runID, g, n, outputs)
	case "end":
		return e.runEnd(ctx, runID, g, n, prompt, outputs)
	default:
		return nil, fmt.Errorf("unknown node type %s", n.Type)
	}
}

func (e *WorkflowEngine) runAgent(ctx context.Context, runID, defID int64, defName string, g *WorkflowGraph, n GraphNode, prompt string, outputs map[string]string) ([]string, error) {
	agentID := g.DataInt64(n, "agent_id")
	if agentID == 0 {
		return nil, fmt.Errorf("agent node missing agent_id")
	}
	a, err := e.svc.Store.GetManagedAgent(ctx, agentID)
	if err != nil || a == nil {
		return nil, fmt.Errorf("agent %d not found", agentID)
	}
	if err := GuardAgentUsable(a); err != nil {
		return nil, err
	}
	attempt, _ := e.svc.Store.NextNodeAttempt(ctx, runID, n.ID)
	ne := &store.NodeExecution{
		RunID: runID, NodeID: n.ID, Attempt: attempt, NodeType: "agent", AgentID: agentID, Status: "running",
		InputJSON: store.MustJSON(map[string]any{"prompt": prompt}),
	}
	execID, err := e.svc.Store.InsertNodeExecution(ctx, ne)
	if err != nil {
		return nil, err
	}

	occ := store.Occupancy{
		AgentID: agentID, SourceType: SourceWorkflow,
		SourceID: strconv.FormatInt(runID, 10), SourceName: defName,
		WorkflowRunID: runID, NodeExecutionID: execID,
		TaskName: truncate(prompt, 120),
	}
	if err := e.svc.TryAcquireOccupancy(ctx, occ); err != nil {
		msg := err.Error()
		if be, ok := err.(*OccupancyBusyError); ok && be.Occ != nil {
			msg = fmt.Sprintf("数字员工忙碌（%s），请终止管理台对话后重试本节点，或重新启动协作并选择执行团队任务", be.Occ.SourceType)
		}
		_ = e.svc.Store.UpdateNodeExecution(ctx, execID, "failed", "", msg, 0)
		return nil, fmt.Errorf("%s", msg)
	}
	defer e.svc.ReleaseOccupancy(context.WithoutCancel(ctx), agentID)

	convID, err := e.svc.GetOrCreateWorkflowAgentConversation(ctx, defID, agentID)
	if err != nil {
		_ = e.svc.Store.UpdateNodeExecution(ctx, execID, "failed", "", err.Error(), 0)
		return nil, err
	}
	// 尽早挂上 conversation，便于前端拉权限
	_ = e.svc.Store.UpdateNodeExecution(ctx, execID, "running", store.MustJSON(map[string]any{
		"status": "RUNNING", "summary": "数字员工执行中…",
	}), "", convID)

	// 节点级「命令自动审核」：开启后本会话走 AI 自动放行（敏感命令仍会 Ask）
	if g.DataBool(n, "auto_review") {
		e.svc.Perms.SetAuto(convID, true)
	}
	// 编排级：允许执行所有命令（rm 除外）
	if g.AllowAllCommands() {
		e.svc.Perms.SetAllowAllExceptRm(convID, true)
	}

	evBuf := newNodeEventBuf(e.svc, execID)
	evBuf.Add(map[string]any{"type": "status", "summary": "节点开始执行", "at": time.Now().UnixMilli()})
	flushCtx, flushCancel := context.WithCancel(ctx)
	defer flushCancel()
	evBuf.StartFlusher(flushCtx)

	// 订阅权限/命令事件写入过程流（裁决仍由前端走 permissions API）
	sub := e.svc.Perms.Subscribe()
	defer e.svc.Perms.Unsubscribe(sub)
	permDone := make(chan struct{})
	go func() {
		defer close(permDone)
		for {
			select {
			case <-flushCtx.Done():
				return
			case pev, ok := <-sub:
				if !ok {
					return
				}
				if pev.Req == nil || pev.Req.ConvID != convID {
					continue
				}
				switch pev.Kind {
				case "request":
					evBuf.Add(map[string]any{
						"type": "permission", "summary": "等待授权 · " + truncate(pev.Req.Summary, 80),
						"tool": pev.Req.ToolName, "at": time.Now().UnixMilli(),
					})
				case "auto":
					evBuf.Add(map[string]any{
						"type": "permission_auto", "summary": "自动放行 · " + truncate(pev.Req.Summary, 80),
						"tool": pev.Req.ToolName, "at": time.Now().UnixMilli(),
					})
				case "command":
					evBuf.Add(map[string]any{
						"type": "command", "summary": truncate(pev.Req.Summary, 100),
						"tool": pev.Req.ToolName, "decision": pev.Req.Decision,
						"risk": pev.Req.Risk, "at": time.Now().UnixMilli(),
					})
				}
			}
		}
	}()

	lastChunkNote := time.Time{}
	onEvent := func(ev map[string]any) {
		ne := askEventToNodeEvent(ev)
		if ne == nil {
			return
		}
		if ne["type"] == "output" {
			if time.Since(lastChunkNote) < 2*time.Second {
				return
			}
			lastChunkNote = time.Now()
		}
		evBuf.Add(ne)
	}

	roleHint := g.DataString(n, "role_hint")
	upstream := CollectDirectUpstream(g, n.ID, outputs)
	q := buildWorkflowAgentPrompt(defName, roleHint, prompt, upstream)
	msg, askErr := e.svc.AskManagedIsolated(ctx, agentID, convID, q, onEvent)
	flushCancel()
	<-permDone
	evBuf.Flush(context.WithoutCancel(ctx))

	text := ""
	if msg != nil {
		text = msg.Content
	}
	if askErr != nil {
		evBuf.Add(map[string]any{"type": "error", "summary": askErr.Error(), "at": time.Now().UnixMilli()})
		evBuf.Flush(context.WithoutCancel(ctx))
		st := "failed"
		errText := askErr.Error()
		if ctx.Err() != nil {
			st = "stopped"
			errText = "用户终止工作流"
		}
		_ = e.svc.Store.UpdateNodeExecution(context.WithoutCancel(ctx), execID, st, store.MustJSON(map[string]any{"text": text, "status": "FAIL"}), errText, convID)
		return nil, askErr
	}
	status := inferPassFail(text)
	nr := BuildAgentNodeResult(text, status)
	out := store.MustJSON(nr)
	outputs[n.ID] = out
	evBuf.Add(map[string]any{"type": "status", "summary": "节点完成 · " + nr.Status, "at": time.Now().UnixMilli()})
	evBuf.Flush(context.WithoutCancel(ctx))
	_ = e.svc.Store.UpdateNodeExecution(ctx, execID, "success", out, "", convID)

	// 写记忆
	_ = e.svc.Memory.Write(context.WithoutCancel(ctx), &store.AgentMemory{
		AgentID: agentID, SourceType: SourceWorkflow, SourceID: strconv.FormatInt(runID, 10),
		WorkflowID: defID, Workspace: a.WorkspacePath,
		Title:   fmt.Sprintf("%s · %s", defName, a.Name),
		Summary: truncate(fmt.Sprintf("任务：%s\n结果：%s", truncate(prompt, 200), nr.Summary), 1200),
		MetadataJSON: store.MustJSON(map[string]any{
			"node_id": n.ID, "run_id": runID, "status": nr.Status,
		}),
	})

	return g.Successors(n.ID), nil
}

func buildWorkflowAgentPrompt(team, role, prompt string, upstream []UpstreamHandoff) string {
	var b strings.Builder
	b.WriteString("【团队工作流任务】\n团队：")
	b.WriteString(team)
	if role != "" {
		b.WriteString("\n你的职责：")
		b.WriteString(role)
	}
	b.WriteString("\n\n用户需求：\n")
	b.WriteString(prompt)
	b.WriteString(FormatUpstreamForPrompt(upstream))
	if len(upstream) == 0 {
		b.WriteString("\n请完成你的部分并给出清晰结论。若可判定通过/失败，请在结尾标明 status=PASS 或 status=FAIL。")
	}
	return b.String()
}

func inferPassFail(text string) string {
	u := strings.ToUpper(text)
	if strings.Contains(u, "STATUS=FAIL") || strings.Contains(u, "STATUS: FAIL") || strings.Contains(text, "失败") && strings.Contains(u, "FAIL") {
		return "FAIL"
	}
	if strings.Contains(u, "STATUS=PASS") || strings.Contains(u, "STATUS: PASS") {
		return "PASS"
	}
	return "PASS"
}

func (e *WorkflowEngine) runHumanReview(ctx context.Context, runID int64, n GraphNode) ([]string, error) {
	attempt, _ := e.svc.Store.NextNodeAttempt(ctx, runID, n.ID)
	ne := &store.NodeExecution{
		RunID: runID, NodeID: n.ID, Attempt: attempt, NodeType: "human_review", Status: "waiting",
		InputJSON: `{}`,
	}
	_, err := e.svc.Store.InsertNodeExecution(ctx, ne)
	if err != nil {
		return nil, err
	}
	return []string{"waiting"}, nil
}

func (e *WorkflowEngine) runCondition(ctx context.Context, runID int64, g *WorkflowGraph, n GraphNode, outputs map[string]string) ([]string, error) {
	attempt, _ := e.svc.Store.NextNodeAttempt(ctx, runID, n.ID)
	expr := g.DataString(n, "expr")
	// AI 预留
	_ = g.DataString(n, "ai_enabled")

	// 合并所有上游输出
	ctxMap := map[string]any{}
	for _, in := range g.ins[n.ID] {
		if raw, ok := outputs[in.Source]; ok {
			for k, v := range FlattenOutput(raw) {
				ctxMap[k] = v
			}
		}
	}
	ok, err := EvalCondition(expr, ctxMap)
	result := map[string]any{"expr": expr, "result": ok, "status": "PASS"}
	if err != nil {
		result["status"] = "FAIL"
		result["error"] = err.Error()
	}
	ne := &store.NodeExecution{
		RunID: runID, NodeID: n.ID, Attempt: attempt, NodeType: "condition", Status: "running",
		InputJSON: store.MustJSON(ctxMap),
	}
	id, ierr := e.svc.Store.InsertNodeExecution(ctx, ne)
	if ierr != nil {
		return nil, ierr
	}
	if err != nil {
		_ = e.svc.Store.UpdateNodeExecution(ctx, id, "failed", store.MustJSON(result), err.Error(), 0)
		return nil, err
	}
	out := store.MustJSON(result)
	// 规范化为可交接 NodeResult，便于 Agent 直接上游读取
	nr := ParseNodeResult(out)
	nr.Summary = truncate(fmt.Sprintf("条件 %s → %v", expr, ok), nodeSummaryMax)
	nr.Result = store.MustJSON(result)
	out = store.MustJSON(nr)
	outputs[n.ID] = out
	_ = e.svc.Store.UpdateNodeExecution(ctx, id, "success", out, "", 0)

	var next []string
	for _, edge := range g.OutEdges(n.ID) {
		br := EdgeBranch(edge)
		if ok && (br == "true" || br == "default" || br == "approve") {
			next = append(next, edge.Target)
		}
		if !ok && (br == "false" || br == "reject") {
			next = append(next, edge.Target)
		}
	}
	// 若无标注分支，true 走第一条，false 走第二条
	if len(next) == 0 {
		edges := g.OutEdges(n.ID)
		if ok && len(edges) > 0 {
			next = append(next, edges[0].Target)
		}
		if !ok && len(edges) > 1 {
			next = append(next, edges[1].Target)
		}
	}
	return next, nil
}

func (e *WorkflowEngine) runParallel(ctx context.Context, runID int64, g *WorkflowGraph, n GraphNode, outputs map[string]string) ([]string, error) {
	attempt, _ := e.svc.Store.NextNodeAttempt(ctx, runID, n.ID)
	ne := &store.NodeExecution{
		RunID: runID, NodeID: n.ID, Attempt: attempt, NodeType: "parallel", Status: "success",
		InputJSON: `{}`, OutputJSON: `{"status":"PASS"}`,
	}
	id, err := e.svc.Store.InsertNodeExecution(ctx, ne)
	if err != nil {
		return nil, err
	}
	_ = e.svc.Store.UpdateNodeExecution(ctx, id, "success", `{"status":"PASS"}`, "", 0)
	outputs[n.ID] = `{"status":"PASS"}`
	return g.Successors(n.ID), nil
}

func (e *WorkflowEngine) runMergePass(ctx context.Context, runID int64, g *WorkflowGraph, n GraphNode, outputs map[string]string) ([]string, error) {
	attempt, _ := e.svc.Store.NextNodeAttempt(ctx, runID, n.ID)
	ne := &store.NodeExecution{
		RunID: runID, NodeID: n.ID, Attempt: attempt, NodeType: "merge", Status: "success",
	}
	id, err := e.svc.Store.InsertNodeExecution(ctx, ne)
	if err != nil {
		return nil, err
	}
	// 合流节点打包所有直接上游，供下游 Agent 一次读到各分支产出
	ups := CollectDirectUpstream(g, n.ID, outputs)
	nr := NodeResult{
		Status:    "PASS",
		Summary:   truncate(fmt.Sprintf("合流完成，合并 %d 个上游分支", len(ups)), nodeSummaryMax),
		Result:    FormatUpstreamForPrompt(ups),
		Artifacts: []NodeArtifact{},
	}
	for _, u := range ups {
		nr.Artifacts = append(nr.Artifacts, u.Result.Artifacts...)
	}
	if len(nr.Artifacts) > maxArtifacts {
		nr.Artifacts = nr.Artifacts[:maxArtifacts]
	}
	out := store.MustJSON(normalizeNodeResult(nr, nr.Result))
	outputs[n.ID] = out
	_ = e.svc.Store.UpdateNodeExecution(ctx, id, "success", out, "", 0)
	return g.Successors(n.ID), nil
}

func (e *WorkflowEngine) runEnd(ctx context.Context, runID int64, g *WorkflowGraph, n GraphNode, prompt string, outputs map[string]string) ([]string, error) {
	attempt, _ := e.svc.Store.NextNodeAttempt(ctx, runID, n.ID)
	summary := map[string]any{"prompt": prompt, "nodes": outputs, "status": "PASS", "text": "工作流结束"}
	ne := &store.NodeExecution{
		RunID: runID, NodeID: n.ID, Attempt: attempt, NodeType: "end", Status: "running",
		InputJSON: store.MustJSON(map[string]any{"prompt": prompt}),
	}
	id, err := e.svc.Store.InsertNodeExecution(ctx, ne)
	if err != nil {
		return nil, err
	}
	out := store.MustJSON(summary)
	outputs[n.ID] = out
	_ = e.svc.Store.UpdateNodeExecution(ctx, id, "success", out, "", 0)
	return nil, nil
}

// ReviewWorkflowRun 人工审核：approve / reject / abort。
func (s *Service) ReviewWorkflowRun(ctx context.Context, runID int64, action, rejectPrompt string) error {
	run, err := s.Store.GetWorkflowRun(ctx, runID)
	if err != nil || run == nil {
		return fmt.Errorf("run not found")
	}
	if run.Status != "waiting" {
		return fmt.Errorf("run not waiting")
	}
	def, _ := s.Store.GetWorkflowDefinition(ctx, run.DefinitionID)
	if def == nil {
		return fmt.Errorf("definition missing")
	}
	g, err := ParseWorkflowGraph(def.GraphJSON)
	if err != nil {
		return err
	}
	// 找 waiting 的 human_review
	list, _ := s.Store.ListNodeExecutions(ctx, runID)
	var waiting *store.NodeExecution
	for i := range list {
		if list[i].Status == "waiting" && (list[i].NodeType == "human_review" || list[i].NodeType == "review") {
			waiting = &list[i]
		}
	}
	if waiting == nil {
		return fmt.Errorf("no waiting review node")
	}
	n, ok := g.Node(waiting.NodeID)
	if !ok {
		return fmt.Errorf("node missing")
	}

	switch strings.ToLower(action) {
	case "abort", "stop":
		s.WF.mu.Lock()
		cancel, ok := s.WF.runs[runID]
		s.WF.mu.Unlock()
		if ok && cancel != nil {
			cancel()
		}
		s.stopWorkflowAgents(ctx, runID)
		_ = s.Store.UpdateNodeExecution(ctx, waiting.ID, "stopped", `{"status":"FAIL"}`, "用户终止", 0)
		return s.Store.UpdateWorkflowRunStatus(ctx, runID, "stopped", waiting.NodeID, "用户终止审核", `[]`, `{}`, `{}`)
	case "reject":
		_ = s.Store.UpdateNodeExecution(ctx, waiting.ID, "failed", store.MustJSON(map[string]any{
			"status": "FAIL", "reject_prompt": rejectPrompt,
		}), "驳回", 0)
		target := g.DataString(n, "reject_target_node_id")
		if target == "" {
			// 默认走 reject 边
			for _, e := range g.OutEdges(n.ID) {
				if EdgeBranch(e) == "reject" || EdgeBranch(e) == "false" {
					target = e.Target
					break
				}
			}
		}
		if target == "" {
			return s.Store.UpdateWorkflowRunStatus(ctx, runID, "failed", waiting.NodeID, "驳回但无回流目标", `[]`, `{}`, `{}`)
		}
		prompt := run.InputPrompt
		if strings.TrimSpace(rejectPrompt) != "" {
			prompt = prompt + "\n\n【审核驳回意见】\n" + rejectPrompt
			_ = s.Store.UpdateWorkflowRunPrompt(ctx, runID, prompt)
		}
		_ = s.Store.UpdateWorkflowRunStatus(ctx, runID, "running", "", "", store.MustJSON([]string{target}), `{}`, `{}`)
		s.resumeFrom(runID, target)
		return nil
	default: // approve
		_ = s.Store.UpdateNodeExecution(ctx, waiting.ID, "success", `{"status":"PASS"}`, "", 0)
		var next string
		for _, e := range g.OutEdges(n.ID) {
			br := EdgeBranch(e)
			if br == "approve" || br == "true" || br == "default" {
				next = e.Target
				break
			}
		}
		if next == "" && len(g.OutEdges(n.ID)) > 0 {
			next = g.OutEdges(n.ID)[0].Target
		}
		if next == "" {
			return s.Store.UpdateWorkflowRunStatus(ctx, runID, "success", "", "", `[]`, `{}`, `{}`)
		}
		_ = s.Store.UpdateWorkflowRunStatus(ctx, runID, "running", "", "", store.MustJSON([]string{next}), `{}`, `{}`)
		s.resumeFrom(runID, next)
		return nil
	}
}

func (s *Service) resumeFrom(runID int64, startNodeID string) {
	s.resumeFromWithOutputs(runID, []string{startNodeID}, nil)
}

// RetryWorkflowRun 从失败节点重试（原参数）。
func (s *Service) RetryWorkflowRun(ctx context.Context, runID int64) error {
	run, err := s.Store.GetWorkflowRun(ctx, runID)
	if err != nil || run == nil {
		return fmt.Errorf("run not found")
	}
	nodeID := run.FailNodeID
	if nodeID == "" {
		return fmt.Errorf("no fail_node_id")
	}
	return s.RetryWorkflowNode(ctx, runID, nodeID, "")
}

// RetryWorkflowNode 按节点重试；extraPrompt 非空则追加「节点补充指令」后重跑该节点及后续。
func (s *Service) RetryWorkflowNode(ctx context.Context, runID int64, nodeID, extraPrompt string) error {
	run, err := s.Store.GetWorkflowRun(ctx, runID)
	if err != nil || run == nil {
		return fmt.Errorf("run not found")
	}
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return fmt.Errorf("node_id required")
	}
	st := run.Status
	if st == "running" || st == "waiting" || st == "pending" {
		return fmt.Errorf("run still active")
	}
	def, err := s.Store.GetWorkflowDefinition(ctx, run.DefinitionID)
	if err != nil || def == nil {
		return fmt.Errorf("definition missing")
	}
	g, err := ParseWorkflowGraph(def.GraphJSON)
	if err != nil {
		return err
	}
	if _, ok := g.Node(nodeID); !ok {
		return fmt.Errorf("node not found")
	}
	if extra := strings.TrimSpace(extraPrompt); extra != "" {
		newPrompt := strings.TrimSpace(run.InputPrompt)
		if newPrompt != "" {
			newPrompt += "\n\n"
		}
		newPrompt += "【节点补充指令 · " + nodeID + "】\n" + extra
		if err := s.Store.UpdateWorkflowRunPrompt(ctx, runID, newPrompt); err != nil {
			return err
		}
	}
	_ = s.Store.UpdateWorkflowRunStatus(ctx, runID, "running", "", "", store.MustJSON([]string{nodeID}), `{}`, `{}`)
	s.resumeFrom(runID, nodeID)
	return nil
}
