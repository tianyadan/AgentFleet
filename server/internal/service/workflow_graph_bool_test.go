package service

import "testing"

func TestDataBool(t *testing.T) {
	g := &WorkflowGraph{}
	n := GraphNode{Data: map[string]any{
		"on": true, "off": false, "one": 1, "zero": 0, "yes": "true", "no": "false",
	}}
	if !g.DataBool(n, "on") || g.DataBool(n, "off") {
		t.Fatal("bool")
	}
	if !g.DataBool(n, "one") || g.DataBool(n, "zero") {
		t.Fatal("int")
	}
	if !g.DataBool(n, "yes") || g.DataBool(n, "no") {
		t.Fatal("string")
	}
	if g.DataBool(n, "missing") {
		t.Fatal("missing should be false")
	}
}
