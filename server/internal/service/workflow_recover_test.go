package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSelfCheckNodeResult_RequiresOutput(t *testing.T) {
	if SelfCheckNodeResult(NodeResult{}, nil) {
		t.Fatal("empty should fail")
	}
	if !SelfCheckNodeResult(NodeResult{Status: "PASS", Summary: "ok", Result: "r"}, nil) {
		t.Fatal("basic should pass")
	}
}

func TestSelfCheckNodeResult_ArtifactPaths(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "out.png")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	ok := SelfCheckNodeResult(NodeResult{
		Status: "PASS", Summary: "s", Result: "r",
		Artifacts: []NodeArtifact{{Type: "image", Ref: f}},
	}, nil)
	if !ok {
		t.Fatal("existing file should pass")
	}
	bad := SelfCheckNodeResult(NodeResult{
		Status: "PASS", Summary: "s", Result: "r",
		Artifacts: []NodeArtifact{{Type: "file", Ref: filepath.Join(dir, "missing.txt")}},
	}, nil)
	if bad {
		t.Fatal("missing file should fail")
	}
	// URL 不强制本地存在
	if !SelfCheckNodeResult(NodeResult{
		Status: "PASS", Summary: "s", Result: "r",
		Artifacts: []NodeArtifact{{Type: "file", Ref: "https://example.com/a.png"}},
	}, nil) {
		t.Fatal("url should pass")
	}
}

func TestPlanResume_ChainReuseThenRerun(t *testing.T) {
	g, err := ParseWorkflowGraph(`{
		"nodes":[
			{"id":"a","type":"agent"},
			{"id":"b","type":"agent"},
			{"id":"c","type":"agent"},
			{"id":"d","type":"agent"}
		],
		"edges":[
			{"id":"e1","source":"a","target":"b"},
			{"id":"e2","source":"b","target":"c"},
			{"id":"e3","source":"c","target":"d"}
		]
	}`)
	if err != nil {
		t.Fatal(err)
	}
	latest := map[string]NodeExecSnapshot{
		"a": {NodeID: "a", Status: "success", OutputJSON: `{"status":"PASS","summary":"A","result":"ra","artifacts":[]}`},
		"b": {NodeID: "b", Status: "success", OutputJSON: `{"status":"PASS","summary":"B","result":"rb","artifacts":[]}`},
		"c": {NodeID: "c", Status: "interrupted", OutputJSON: `{"status":"FAIL","summary":"半成品","result":"","artifacts":[]}`},
	}
	plan := PlanResume(g, latest, nil)
	if !plan.Reusable["a"] || !plan.Reusable["b"] {
		t.Fatalf("A/B should reuse: %+v", plan.Reusable)
	}
	if plan.Reusable["c"] {
		t.Fatal("C should not reuse")
	}
	if len(plan.Roots) != 1 || plan.Roots[0] != "c" {
		t.Fatalf("roots want [c], got %v", plan.Roots)
	}
	if plan.Outputs["a"] == "" || plan.Outputs["b"] == "" {
		t.Fatal("outputs for reusable missing")
	}
}

func TestPlanResume_ParallelKeepSuccessSibling(t *testing.T) {
	g, err := ParseWorkflowGraph(`{
		"nodes":[
			{"id":"p","type":"parallel"},
			{"id":"b","type":"agent"},
			{"id":"c","type":"agent"},
			{"id":"m","type":"merge"}
		],
		"edges":[
			{"id":"e1","source":"p","target":"b"},
			{"id":"e2","source":"p","target":"c"},
			{"id":"e3","source":"b","target":"m"},
			{"id":"e4","source":"c","target":"m"}
		]
	}`)
	if err != nil {
		t.Fatal(err)
	}
	latest := map[string]NodeExecSnapshot{
		"p": {NodeID: "p", Status: "success", OutputJSON: `{"status":"PASS","summary":"fan","result":"ok","artifacts":[]}`},
		"b": {NodeID: "b", Status: "success", OutputJSON: `{"status":"PASS","summary":"B","result":"rb","artifacts":[]}`},
		"c": {NodeID: "c", Status: "interrupted", OutputJSON: `{}`},
		"m": {NodeID: "m", Status: "success", OutputJSON: `{"status":"PASS","summary":"merge","result":"old","artifacts":[]}`},
	}
	plan := PlanResume(g, latest, nil)
	if !plan.Reusable["b"] {
		t.Fatal("B should stay reused")
	}
	if plan.Reusable["c"] || plan.Reusable["m"] {
		t.Fatalf("C and merge must rerun: reusable=%v", plan.Reusable)
	}
	if len(plan.Roots) != 1 || plan.Roots[0] != "c" {
		t.Fatalf("roots want [c], got %v", plan.Roots)
	}
}

func TestPlanResume_MissingArtifactInvalidatesSuccess(t *testing.T) {
	g, _ := ParseWorkflowGraph(`{
		"nodes":[{"id":"a","type":"agent"},{"id":"b","type":"agent"}],
		"edges":[{"id":"e","source":"a","target":"b"}]
	}`)
	latest := map[string]NodeExecSnapshot{
		"a": {NodeID: "a", Status: "success", OutputJSON: `{"status":"PASS","summary":"A","result":"ra","artifacts":[{"type":"file","ref":"/no/such/path/zzz.bin"}]}`},
		"b": {NodeID: "b", Status: "interrupted", OutputJSON: `{}`},
	}
	plan := PlanResume(g, latest, nil)
	if plan.Reusable["a"] {
		t.Fatal("A missing artifact should not reuse")
	}
	// A needRerun → B also needRerun; roots should include A (no pred in needRerun)
	foundA := false
	for _, r := range plan.Roots {
		if r == "a" {
			foundA = true
		}
	}
	if !foundA {
		t.Fatalf("roots should include a, got %v", plan.Roots)
	}
}

func TestIsFilesystemArtifactRef(t *testing.T) {
	if !isFilesystemArtifactRef("./a.png") || !isFilesystemArtifactRef("/tmp/x") {
		t.Fatal("local paths")
	}
	if isFilesystemArtifactRef("https://x.com/a") || isFilesystemArtifactRef("") {
		t.Fatal("non-fs")
	}
	_ = strings.TrimSpace
}
