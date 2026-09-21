package service

import "colleague-avatar/server/internal/store"

// CollectWorkflowStopAgentIDs 收集应随工作流终止而停掉的数字员工（占用 + 仍在跑的 agent 节点）。
func CollectWorkflowStopAgentIDs(runID int64, occ map[int64]store.Occupancy, nes []store.NodeExecution) []int64 {
	seen := map[int64]bool{}
	var ids []int64
	add := func(id int64) {
		if id <= 0 || seen[id] {
			return
		}
		seen[id] = true
		ids = append(ids, id)
	}
	for _, o := range occ {
		if o.WorkflowRunID == runID && o.SourceType == SourceWorkflow {
			add(o.AgentID)
		}
	}
	for _, ne := range nes {
		if ne.AgentID <= 0 {
			continue
		}
		st := ne.Status
		if st == "running" || st == "waiting" {
			add(ne.AgentID)
		}
	}
	return ids
}
