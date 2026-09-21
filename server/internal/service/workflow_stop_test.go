package service

import (
	"testing"

	"colleague-avatar/server/internal/store"
)

func TestCollectWorkflowStopAgentIDs(t *testing.T) {
	occ := map[int64]store.Occupancy{
		11: {AgentID: 11, WorkflowRunID: 9, SourceType: SourceWorkflow},
		12: {AgentID: 12, WorkflowRunID: 8, SourceType: SourceWorkflow},
		13: {AgentID: 13, WorkflowRunID: 9, SourceType: SourceDirect},
	}
	nes := []store.NodeExecution{
		{AgentID: 21, Status: "running", NodeType: "agent"},
		{AgentID: 22, Status: "waiting", NodeType: "agent"},
		{AgentID: 0, Status: "running", NodeType: "condition"},
		{AgentID: 11, Status: "success", NodeType: "agent"},
	}
	got := CollectWorkflowStopAgentIDs(9, occ, nes)
	want := map[int64]bool{11: true, 21: true, 22: true}
	if len(got) != len(want) {
		t.Fatalf("ids=%v want %v", got, want)
	}
	for _, id := range got {
		if !want[id] {
			t.Fatalf("unexpected id %d in %v", id, got)
		}
	}
}
