package service

import (
	"testing"

	"colleague-avatar/server/internal/store"
)

func TestCollectAgentIDsFromGraph(t *testing.T) {
	g, err := ParseWorkflowGraph(`{
	  "nodes":[
	    {"id":"s","type":"start","data":{}},
	    {"id":"a1","type":"agent","data":{"agent_id":11}},
	    {"id":"a2","type":"agent","data":{"agent_id":22}},
	    {"id":"a3","type":"agent","data":{"agent_id":11}},
	    {"id":"e","type":"end","data":{}}
	  ],
	  "edges":[]
	}`)
	if err != nil {
		t.Fatal(err)
	}
	ids := CollectAgentIDsFromGraph(g)
	if len(ids) != 2 || !ids[11] || !ids[22] {
		t.Fatalf("got %#v", ids)
	}
}

func TestFilterStartConflicts(t *testing.T) {
	agentIDs := map[int64]bool{1: true, 2: true, 3: true}
	occ := map[int64]store.Occupancy{
		1: {AgentID: 1, SourceType: SourceDirect, TaskName: "聊天"},
		2: {AgentID: 2, SourceType: SourceCloneInit, TaskName: "整理"},
		3: {AgentID: 3, SourceType: SourceWorkflow, WorkflowRunID: 99, TaskName: "别的协作"},
		4: {AgentID: 4, SourceType: SourceDirect}, // 不在图中
	}
	names := map[int64]string{1: "甲", 2: "乙", 3: "丙"}
	got := FilterStartConflicts(agentIDs, occ, names)
	if len(got) != 3 {
		t.Fatalf("want 3 conflicts, got %d %#v", len(got), got)
	}
	if FilterStartConflicts(agentIDs, map[int64]store.Occupancy{}, names) != nil {
		t.Fatal("empty occ")
	}
}

func TestIsWorkflowStartConflict(t *testing.T) {
	if !IsWorkflowStartConflictError(&WorkflowStartConflictError{Conflicts: []AgentConflict{{AgentID: 1}}}) {
		t.Fatal("want true")
	}
	if IsWorkflowStartConflictError(ErrAgentInitializing) {
		t.Fatal("want false")
	}
}
