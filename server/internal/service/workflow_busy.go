package service

import (
	"fmt"

	"colleague-avatar/server/internal/store"
)

// AgentConflict 启动协作时与现有占用冲突的员工。
type AgentConflict struct {
	AgentID    int64            `json:"agent_id"`
	Name       string           `json:"name"`
	Occupancy  *store.Occupancy `json:"occupancy"`
}

// WorkflowStartConflictError 需前端弹窗二选一。
type WorkflowStartConflictError struct {
	Conflicts []AgentConflict `json:"conflicts"`
}

func (e *WorkflowStartConflictError) Error() string {
	if e == nil || len(e.Conflicts) == 0 {
		return "数字员工忙碌"
	}
	if len(e.Conflicts) == 1 {
		c := e.Conflicts[0]
		return fmt.Sprintf("数字员工「%s」正在忙碌", c.Name)
	}
	return fmt.Sprintf("有 %d 名数字员工正在忙碌", len(e.Conflicts))
}

// IsWorkflowStartConflictError 是否为启动冲突。
func IsWorkflowStartConflictError(err error) bool {
	_, ok := err.(*WorkflowStartConflictError)
	return ok
}

// CollectAgentIDsFromGraph 收集图中全部 agent 节点的员工 id。
func CollectAgentIDsFromGraph(g *WorkflowGraph) map[int64]bool {
	out := map[int64]bool{}
	if g == nil {
		return out
	}
	for _, n := range g.Nodes {
		if n.Type != "agent" {
			continue
		}
		id := g.DataInt64(n, "agent_id")
		if id > 0 {
			out[id] = true
		}
	}
	return out
}

// FilterStartConflicts 图中员工若已被占用（任意 source），则构成启动冲突。
func FilterStartConflicts(agentIDs map[int64]bool, occ map[int64]store.Occupancy, names map[int64]string) []AgentConflict {
	var out []AgentConflict
	for id := range agentIDs {
		o, ok := occ[id]
		if !ok {
			continue
		}
		oc := o
		name := names[id]
		if name == "" {
			name = fmt.Sprintf("#%d", id)
		}
		out = append(out, AgentConflict{AgentID: id, Name: name, Occupancy: &oc})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
