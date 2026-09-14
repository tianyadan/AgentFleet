package service

import (
	"strings"
	"testing"
)

func TestCollectDirectUpstream_ChainOnlyImmediate(t *testing.T) {
	g, err := ParseWorkflowGraph(`{
		"nodes":[
			{"id":"a","type":"agent"},
			{"id":"b","type":"agent"},
			{"id":"c","type":"agent"}
		],
		"edges":[
			{"id":"e1","source":"a","target":"b"},
			{"id":"e2","source":"b","target":"c"}
		]
	}`)
	if err != nil {
		t.Fatal(err)
	}
	outputs := map[string]string{
		"a": `{"status":"PASS","summary":"A做完","result":"A结果","artifacts":[]}`,
		"b": `{"status":"PASS","summary":"B做完","result":"B结果","artifacts":[]}`,
	}
	ups := CollectDirectUpstream(g, "c", outputs)
	if len(ups) != 1 || ups[0].SourceNodeID != "b" {
		t.Fatalf("C should only get B, got %+v", ups)
	}
	if ups[0].Result.Result != "B结果" {
		t.Fatalf("result=%q", ups[0].Result.Result)
	}
	upsB := CollectDirectUpstream(g, "b", outputs)
	if len(upsB) != 1 || upsB[0].SourceNodeID != "a" {
		t.Fatalf("B should only get A, got %+v", upsB)
	}
}

func TestCollectDirectUpstream_FanIn(t *testing.T) {
	g, err := ParseWorkflowGraph(`{
		"nodes":[
			{"id":"a","type":"agent"},
			{"id":"b","type":"agent"},
			{"id":"m","type":"merge"}
		],
		"edges":[
			{"id":"e1","source":"a","target":"m"},
			{"id":"e2","source":"b","target":"m"}
		]
	}`)
	if err != nil {
		t.Fatal(err)
	}
	outputs := map[string]string{
		"a": `{"status":"PASS","summary":"A","result":"ra","artifacts":[]}`,
		"b": `{"status":"PASS","summary":"B","result":"rb","artifacts":[]}`,
	}
	ups := CollectDirectUpstream(g, "m", outputs)
	if len(ups) != 2 {
		t.Fatalf("want 2 upstream, got %d", len(ups))
	}
	ids := map[string]bool{}
	for _, u := range ups {
		ids[u.SourceNodeID] = true
	}
	if !ids["a"] || !ids["b"] {
		t.Fatalf("missing sources: %+v", ups)
	}
}

func TestFormatUpstreamForPrompt_InstructsNoRedo(t *testing.T) {
	s := FormatUpstreamForPrompt([]UpstreamHandoff{{
		SourceNodeID: "ui",
		SourceType:   "agent",
		Result: NodeResult{
			Status: "PASS", Summary: "已完成设计稿", Result: "设计说明…",
			Artifacts: []NodeArtifact{{Type: "image", Ref: "./out.png"}},
		},
	}})
	for _, want := range []string{"直接上游", "不要重复", "ui", "已完成设计稿", "设计说明", "./out.png", "优先使用"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in:\n%s", want, s)
		}
	}
}

func TestBuildAgentNodeResult_Structured(t *testing.T) {
	nr := BuildAgentNodeResult("完成了登录页。\n\n产物: ./login.png\nstatus=PASS", "PASS")
	if nr.Status != "PASS" {
		t.Fatalf("status=%s", nr.Status)
	}
	if nr.Summary == "" || nr.Result == "" {
		t.Fatalf("empty summary/result: %+v", nr)
	}
	if len(nr.Artifacts) == 0 {
		t.Fatalf("expected artifact from path, got %+v", nr)
	}
}

func TestBuildWorkflowAgentPrompt_OnlyDirectUpstream(t *testing.T) {
	g, _ := ParseWorkflowGraph(`{
		"nodes":[{"id":"a","type":"agent"},{"id":"b","type":"agent"}],
		"edges":[{"id":"e","source":"a","target":"b"}]
	}`)
	outputs := map[string]string{
		"a": `{"status":"PASS","summary":"上游摘要","result":"上游核心","artifacts":[]}`,
		"x": `{"status":"PASS","summary":"无关节点","result":"不应出现","artifacts":[]}`,
	}
	ups := CollectDirectUpstream(g, "b", outputs)
	p := buildWorkflowAgentPrompt("团队", "做前端", "做个网站", ups)
	if !strings.Contains(p, "上游核心") || !strings.Contains(p, "上游摘要") {
		t.Fatalf("missing upstream: %s", p)
	}
	if strings.Contains(p, "不应出现") || strings.Contains(p, "无关节点") {
		t.Fatalf("leaked non-direct upstream: %s", p)
	}
	if !strings.Contains(p, "不要重复") {
		t.Fatalf("missing no-redo instruction")
	}
}

func TestParseNodeResult_LegacyCompatible(t *testing.T) {
	nr := ParseNodeResult(`{"text":"旧格式正文","status":"PASS","summary":"旧摘要"}`)
	if nr.Result != "旧格式正文" || nr.Summary != "旧摘要" {
		t.Fatalf("%+v", nr)
	}
}
