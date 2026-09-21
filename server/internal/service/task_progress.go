package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"colleague-avatar/server/internal/store"
)

// TaskProgress 管理某轮 Ask 的进度上报(agent_type=managed)。
type TaskProgress struct {
	svc     *Service
	agentID int64
	taskID  string
	name    string
	steps   []string
	idx     int
}

func (s *Service) beginTaskProgress(ctx context.Context, agentID int64, taskName string, steps []string) *TaskProgress {
	if len(steps) == 0 {
		steps = HeuristicPlanSteps(taskName, nil)
	}
	tp := &TaskProgress{
		svc: s, agentID: agentID,
		taskID: fmt.Sprintf("run-%d-%d", agentID, time.Now().UnixNano()),
		name:   taskName,
		steps:  steps,
	}
	_ = tp.report(ctx, 0, "running", steps[0])
	return tp
}

func (tp *TaskProgress) report(ctx context.Context, stepIdx int, status, msg string) error {
	if tp == nil || tp.svc == nil {
		return nil
	}
	n := len(tp.steps)
	if n == 0 {
		n = 1
	}
	if stepIdx < 0 {
		stepIdx = 0
	}
	if stepIdx >= n {
		stepIdx = n - 1
	}
	tp.idx = stepIdx
	prog := ((stepIdx + 1) * 100) / n
	if status == "done" {
		prog = 100
	}
	if status == "running" && prog > 95 {
		prog = 95
	}
	if msg == "" && stepIdx < len(tp.steps) {
		msg = tp.steps[stepIdx]
	}
	_, err := tp.svc.Store.UpsertAgentTask(ctx, store.AgentTaskReport{
		AgentType: "managed",
		AgentID:   strconv.FormatInt(tp.agentID, 10),
		TaskID:    tp.taskID,
		TaskName:  tp.name,
		Status:    status,
		Progress:  prog,
		Message:   msg,
		Payload: map[string]any{
			"steps":        tp.steps,
			"current_step": stepIdx,
		},
	})
	return err
}

func (tp *TaskProgress) advance(ctx context.Context, stepIdx int, msg string) {
	_ = tp.report(ctx, stepIdx, "running", msg)
}

func (tp *TaskProgress) done(ctx context.Context, msg string) {
	_ = tp.report(ctx, len(tp.steps)-1, "done", msg)
}

func (tp *TaskProgress) fail(ctx context.Context, msg string) {
	if tp == nil || tp.svc == nil {
		return
	}
	n := len(tp.steps)
	if n == 0 {
		n = 1
	}
	stepIdx := tp.idx
	if stepIdx < 0 {
		stepIdx = 0
	}
	if stepIdx >= n {
		stepIdx = n - 1
	}
	prog := ((stepIdx + 1) * 100) / n
	if prog > 95 {
		prog = 95
	}
	if msg == "" {
		msg = "步骤失败"
	}
	_, _ = tp.svc.Store.UpsertAgentTask(ctx, store.AgentTaskReport{
		AgentType: "managed",
		AgentID:   strconv.FormatInt(tp.agentID, 10),
		TaskID:    tp.taskID,
		TaskName:  tp.name,
		Status:    "failed",
		Progress:  prog,
		Message:   msg,
		Payload: map[string]any{
			"steps":        tp.steps,
			"current_step": stepIdx,
			"failed_step":  stepIdx,
			"failed":       true,
		},
	})
}

func truncateRunTitle(q string) string {
	q = strings.TrimSpace(q)
	if q == "" {
		return "数字员工任务"
	}
	return truncateRunes(q, 40)
}
