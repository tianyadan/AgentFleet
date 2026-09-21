package service

import "colleague-avatar/server/internal/store"

// shouldEmitTaskPlan 是否对本轮 Ask 做 AI 任务规划与进度上报（默认开）。
func shouldEmitTaskPlan(a *store.ManagedAgent) bool {
	if a == nil {
		return true
	}
	return a.TaskPlanEnabled
}
