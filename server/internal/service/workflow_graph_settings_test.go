package service

import "testing"

func TestWorkflowGraphAllowAllCommands(t *testing.T) {
	g := &WorkflowGraph{Settings: map[string]any{"allow_all_commands": true}}
	if !g.AllowAllCommands() {
		t.Fatal("expected allow all")
	}
	g2 := &WorkflowGraph{Settings: map[string]any{"allow_all_commands": "yes"}}
	if !g2.AllowAllCommands() {
		t.Fatal("string yes should allow")
	}
	g3 := &WorkflowGraph{}
	if g3.AllowAllCommands() {
		t.Fatal("nil settings should deny")
	}
}

func TestCopyStringMap(t *testing.T) {
	src := map[string]string{"a": "1"}
	dst := copyStringMap(src)
	dst["a"] = "2"
	if src["a"] != "1" {
		t.Fatal("copy should isolate")
	}
}
