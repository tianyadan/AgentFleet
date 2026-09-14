package service

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"colleague-avatar/server/internal/store"
)

// NodeExecSnapshot 恢复规划用的节点最新 attempt 摘要。
type NodeExecSnapshot struct {
	NodeID     string
	Attempt    int
	Status     string
	OutputJSON string
	ErrorText  string
}

// ResumePlan 恢复执行计划。
type ResumePlan struct {
	Reusable map[string]bool
	NeedRerun map[string]bool
	Outputs  map[string]string // 可复用节点的 output_json
	Roots    []string          // 需重跑且无 needRerun 前驱的起点
}

// PathExistsFn 产物路径是否存在（测试可注入）。
type PathExistsFn func(path string) bool

func defaultPathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// SelfCheckNodeResult 轻量自检：有有效 output；本地路径产物需仍存在。
func SelfCheckNodeResult(nr NodeResult, exists PathExistsFn) bool {
	if exists == nil {
		exists = defaultPathExists
	}
	if strings.TrimSpace(nr.Summary) == "" && strings.TrimSpace(nr.Result) == "" && strings.TrimSpace(nr.Text) == "" {
		return false
	}
	for _, a := range nr.Artifacts {
		ref := strings.TrimSpace(a.Ref)
		if !isFilesystemArtifactRef(ref) {
			continue
		}
		if !exists(ref) {
			return false
		}
	}
	return true
}

func isFilesystemArtifactRef(ref string) bool {
	if ref == "" {
		return false
	}
	low := strings.ToLower(ref)
	if strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") || strings.HasPrefix(low, "data:") {
		return false
	}
	return strings.HasPrefix(ref, "/") ||
		strings.HasPrefix(ref, "./") ||
		strings.HasPrefix(ref, "../") ||
		strings.HasPrefix(ref, "~/") ||
		(len(ref) > 2 && ref[1] == ':' && (ref[2] == '\\' || ref[2] == '/'))
}

// PlanResume 根据最新 attempt 计算可复用集合与重跑起点。
func PlanResume(g *WorkflowGraph, latest map[string]NodeExecSnapshot, exists PathExistsFn) ResumePlan {
	plan := ResumePlan{
		Reusable:  map[string]bool{},
		NeedRerun: map[string]bool{},
		Outputs:   map[string]string{},
	}
	if g == nil {
		return plan
	}
	for id, snap := range latest {
		nr := ParseNodeResult(snap.OutputJSON)
		if snap.Status == "success" && SelfCheckNodeResult(nr, exists) {
			plan.Reusable[id] = true
			plan.Outputs[id] = snap.OutputJSON
			continue
		}
		// 有过执行记录但不可复用 → 需重跑
		if snap.Status != "" {
			plan.NeedRerun[id] = true
		}
	}
	// 传播：前驱需重跑则 success 后继也不可复用
	changed := true
	for changed {
		changed = false
		for _, n := range g.Nodes {
			if !plan.Reusable[n.ID] {
				continue
			}
			for _, e := range g.ins[n.ID] {
				if plan.NeedRerun[e.Source] {
					delete(plan.Reusable, n.ID)
					delete(plan.Outputs, n.ID)
					plan.NeedRerun[n.ID] = true
					changed = true
					break
				}
			}
		}
	}
	// 从未执行、但所有前驱已可复用且存在后继推进需求时：由 roots 执行后自然进入
	// roots = needRerun 且无 needRerun 前驱
	for id := range plan.NeedRerun {
		hasBadPred := false
		for _, e := range g.ins[id] {
			if plan.NeedRerun[e.Source] {
				hasBadPred = true
				break
			}
		}
		if !hasBadPred {
			plan.Roots = append(plan.Roots, id)
		}
	}
	// 稳定顺序：按图中 nodes 顺序
	if len(plan.Roots) > 1 {
		order := map[string]int{}
		for i, n := range g.Nodes {
			order[n.ID] = i
		}
		roots := plan.Roots
		for i := 0; i < len(roots); i++ {
			for j := i + 1; j < len(roots); j++ {
				if order[roots[j]] < order[roots[i]] {
					roots[i], roots[j] = roots[j], roots[i]
				}
			}
		}
		plan.Roots = roots
	}
	// 无 needRerun：从「可复用 frontier」的后继继续（例如只跑到一半但最后节点 success 自检挂了已处理）
	if len(plan.NeedRerun) == 0 {
		plan.Roots = frontierSuccessors(g, plan.Reusable)
	}
	return plan
}

// frontierSuccessors 所有可复用节点的、尚未可复用的直接后继（去重）。
func frontierSuccessors(g *WorkflowGraph, reusable map[string]bool) []string {
	seen := map[string]bool{}
	var out []string
	for id := range reusable {
		for _, e := range g.outs[id] {
			if reusable[e.Target] || seen[e.Target] {
				continue
			}
			seen[e.Target] = true
			out = append(out, e.Target)
		}
	}
	return out
}

// LatestNodeSnapshots 按 node_id 取最高 attempt。
func LatestNodeSnapshots(list []NodeExecSnapshot) map[string]NodeExecSnapshot {
	out := map[string]NodeExecSnapshot{}
	for _, ne := range list {
		prev, ok := out[ne.NodeID]
		if !ok || ne.Attempt >= prev.Attempt {
			out[ne.NodeID] = ne
		}
	}
	return out
}

const workflowRecoveryWindow = 3 * time.Minute

// BeginWorkflowRecoveryWindow 启动后自动恢复 waiting_recovery；窗口结束仍未恢复则 interrupted。
func (s *Service) BeginWorkflowRecoveryWindow() {
	go func() {
		deadline := time.Now().Add(workflowRecoveryWindow)
		ctx := context.Background()
		for {
			runs, err := s.Store.ListWorkflowRunsByStatus(ctx, "waiting_recovery", 200)
			if err != nil {
				log.Printf("list waiting_recovery: %v", err)
			} else {
				for _, r := range runs {
					if err := s.ResumeWorkflowRun(ctx, r.ID); err != nil {
						log.Printf("auto resume workflow %d: %v", r.ID, err)
					}
				}
			}
			if time.Now().After(deadline) {
				if n, err := s.Store.MarkWaitingRecoveryInterrupted(ctx, "恢复窗口超时（3分钟），未能自动续跑"); err != nil {
					log.Printf("mark waiting_recovery interrupted: %v", err)
				} else if n > 0 {
					log.Printf("marked %d waiting_recovery runs as interrupted after timeout", n)
				}
				return
			}
			// 窗口内每 5 秒再扫一次尚未踢出 waiting_recovery 的 run
			time.Sleep(5 * time.Second)
		}
	}()
}

// ResumeWorkflowRun 自检并续跑（waiting_recovery / interrupted）。
func (s *Service) ResumeWorkflowRun(ctx context.Context, runID int64) error {
	run, err := s.Store.GetWorkflowRun(ctx, runID)
	if err != nil || run == nil {
		return fmt.Errorf("run not found")
	}
	st := run.Status
	if st != "waiting_recovery" && st != "interrupted" {
		return fmt.Errorf("run status %s cannot resume", st)
	}
	s.WF.mu.Lock()
	_, busy := s.WF.runs[runID]
	s.WF.mu.Unlock()
	if busy {
		return fmt.Errorf("run already active")
	}
	def, err := s.Store.GetWorkflowDefinition(ctx, run.DefinitionID)
	if err != nil || def == nil {
		return fmt.Errorf("definition missing")
	}
	g, err := ParseWorkflowGraph(def.GraphJSON)
	if err != nil {
		return err
	}
	list, err := s.Store.ListNodeExecutions(ctx, runID)
	if err != nil {
		return err
	}
	snaps := make([]NodeExecSnapshot, 0, len(list))
	for _, ne := range list {
		snaps = append(snaps, NodeExecSnapshot{
			NodeID: ne.NodeID, Attempt: ne.Attempt, Status: ne.Status,
			OutputJSON: ne.OutputJSON, ErrorText: ne.ErrorText,
		})
	}
	plan := PlanResume(g, LatestNodeSnapshots(snaps), nil)
	if len(plan.Roots) == 0 {
		// 全部可复用：若存在 end 成功或图已走完则标 success，否则 interrupted
		if endReusable(g, plan.Reusable) {
			_ = s.Store.UpdateWorkflowRunStatus(ctx, runID, "success", "", "", `[]`, `{}`, `{}`)
			return nil
		}
		_ = s.Store.UpdateWorkflowRunStatus(ctx, runID, "interrupted", "", "恢复自检后无可推进节点", `[]`, `{}`, `{}`)
		return fmt.Errorf("no resume roots")
	}
	_ = s.Store.UpdateWorkflowRunStatus(ctx, runID, "running", "", "", store.MustJSON(plan.Roots), `{}`, `{}`)
	s.resumeFromWithOutputs(runID, plan.Roots, plan.Outputs)
	return nil
}

func endReusable(g *WorkflowGraph, reusable map[string]bool) bool {
	for _, n := range g.Nodes {
		if n.Type == "end" && reusable[n.ID] {
			return true
		}
	}
	return false
}

func (s *Service) resumeFromWithOutputs(runID int64, starts []string, seed map[string]string) {
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
		_ = s.WF.executeFromWithSeed(runCtx, runID, starts, seed)
	}()
}
