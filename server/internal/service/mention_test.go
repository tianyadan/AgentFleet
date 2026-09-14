package service

import "testing"

func TestHeuristicPlanStepsConcrete(t *testing.T) {
	steps := HeuristicPlanSteps("帮我查登录接口超时原因，并给出修复建议", nil)
	if len(steps) < 4 {
		t.Fatalf("want >=4 steps, got %v", steps)
	}
	for _, s := range steps {
		if vagueStepRe.MatchString(s) {
			t.Fatalf("vague step: %s", s)
		}
	}
}

func TestFilterDropsVague(t *testing.T) {
	got := filterConcreteSteps([]string{"正在规划任务", "查阅 auth 中间件超时配置", "正在执行中", "修改重试次数并回归"})
	if len(got) != 2 {
		t.Fatalf("got %v", got)
	}
}

func TestParseMentionsTokenAndName(t *testing.T) {
	agents := []AgentRef{
		{ID: 6, Name: "Codex助手"},
		{ID: 7, Name: "Claude审"},
		{ID: 8, Name: "Cursor"},
	}
	q := `请 @[Codex助手](#agent:6) 查日志，再请 @[Claude审](#agent:7) 审一下。顺便提一下 @nobody`
	ms, rest := ParseMentions(q, agents)
	if len(ms) != 2 {
		t.Fatalf("want 2 mentions, got %d %+v", len(ms), ms)
	}
	if ms[0].ID != 6 || ms[1].ID != 7 {
		t.Fatalf("ids: %+v", ms)
	}
	if rest == q {
		t.Fatal("rest should strip mention tokens")
	}
	if !ContainsAgentID(ms, 6) || ContainsAgentID(ms, 8) {
		t.Fatal("ContainsAgentID")
	}
}

func TestParseMentionsDedup(t *testing.T) {
	agents := []AgentRef{{ID: 1, Name: "甲"}, {ID: 2, Name: "乙"}}
	ms, _ := ParseMentions(`@[甲](#agent:1) 和 @[甲](#agent:1) 还有 @乙`, agents)
	if len(ms) != 2 {
		t.Fatalf("dedup want 2 got %d", len(ms))
	}
}
