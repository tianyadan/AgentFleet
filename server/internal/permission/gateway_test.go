package permission

import (
	"context"
	"testing"
)

func TestHardDenyRootRmNotRelative(t *testing.T) {
	if !IsHardDeny(FromShell("claude", 1, 1, "rm -rf /", "/ws", "")) {
		t.Fatal("rm -rf / must hard deny")
	}
	if !IsHardDeny(FromShell("claude", 1, 1, "sudo rm -rf /*", "/ws", "")) {
		t.Fatal("sudo rm -rf /* must hard deny")
	}
	if IsHardDeny(FromShell("claude", 1, 1, "rm -rf ./dist", "/ws", "")) {
		t.Fatal("rm -rf ./dist must not hard deny")
	}
}

func TestHardDenyDiskAndSQL(t *testing.T) {
	for _, cmd := range []string{
		"mkfs.ext4 /dev/sda",
		"dd if=/dev/zero of=/dev/sda bs=1M",
		"wipefs -a /dev/nvme0n1",
		"fdisk /dev/sda",
		"parted /dev/sda rm 1",
		`psql -c "DROP DATABASE prod"`,
		"chmod -R 777 /",
	} {
		if !IsHardDeny(FromShell("codex", 1, 1, cmd, "/opt", "production")) {
			t.Fatalf("want hard deny: %s", cmd)
		}
	}
}

func TestSafeAllowReadOnly(t *testing.T) {
	for _, cmd := range []string{"ls", "pwd", "cat README.md", "head -n 20 a.txt", "grep foo x", "git status", "git log", "git diff", "git branch"} {
		a := FromShell("claude", 1, 1, cmd, "/ws", "")
		if !IsSafeAllow(a) {
			t.Fatalf("want safe allow: %s type=%s", cmd, a.ActionType)
		}
	}
	if IsSafeAllow(FromShell("claude", 1, 1, "git commit -m x", "/ws", "")) {
		t.Fatal("git commit is not safe allow")
	}
}

func TestNormalizeSignatureExact(t *testing.T) {
	a := FromShell("claude", 1, 1, "  sudo  npm   install   axios ", "/opt/p", "dev")
	b := FromShell("claude", 1, 1, "npm install axios", "/opt/p", "dev")
	if a.Signature() != b.Signature() {
		t.Fatalf("normalize mismatch\n%s\n%s", a.Signature(), b.Signature())
	}
	c := FromShell("claude", 1, 1, "npm install lodash", "/opt/p", "dev")
	if a.Signature() == c.Signature() {
		t.Fatal("different packages must not share signature")
	}
}

func TestGatewayHardDenyBeatsSession(t *testing.T) {
	mem := NewMemorySessions()
	g := NewGateway(mem, nil)
	bad := FromShell("cursor", 9, 2, "rm -rf /", "/ws", "")
	_ = mem.Grant(9, SessionGrantFrom(bad))
	d := g.Evaluate(nil, bad, EvaluateOpts{})
	if d.Decision != DecisionDeny || d.DecidedBy != DecidedByStatic {
		t.Fatalf("got %+v", d)
	}
}

func TestGatewaySessionAllowExact(t *testing.T) {
	mem := NewMemorySessions()
	g := NewGateway(mem, nil)
	a := FromShell("claude", 3, 1, "npm install axios", "/app", "dev")
	_ = mem.Grant(3, SessionGrantFrom(a))
	d := g.Evaluate(nil, a, EvaluateOpts{})
	if d.Decision != DecisionAllow || d.DecidedBy != DecidedBySession || !d.SessionAuthorized {
		t.Fatalf("got %+v", d)
	}
	other := FromShell("claude", 3, 1, "npm install webpack", "/app", "dev")
	d2 := g.Evaluate(nil, other, EvaluateOpts{})
	if d2.DecidedBy == DecidedBySession {
		t.Fatal("webpack must not inherit axios grant")
	}
}

func TestJevosScoreBands(t *testing.T) {
	// 旧单指标边界仍保留（用于兼容）
	if DecisionFromRisk(0.45) != DecisionAllow {
		t.Fatal("0.45 allow")
	}
	if DecisionFromRisk(0.46) != DecisionReview {
		t.Fatal("0.46 review")
	}
	if DecisionFromRisk(0.59) != DecisionReview {
		t.Fatal("0.59 review")
	}
	if DecisionFromRisk(0.60) != DecisionDeny {
		t.Fatal("0.60 deny")
	}
}

// needs_review 高分只应人工复核，不得因「需要审核」直接拒绝。
func TestDecisionFromJevosNeedsReviewIsNotDeny(t *testing.T) {
	jr := JEVOSResult{
		Destructive: 0.2, DataLoss: 0.1, ServiceImpact: 0.1,
		PermissionRisk: 0.2, CredentialRisk: 0.1, NeedsReview: 0.9,
		RiskScore: 0.9,
	}
	if DecisionFromJevos(jr) != DecisionReview {
		t.Fatalf("needs_review high must REVIEW, got %s", DecisionFromJevos(jr))
	}
}

// 明确高危害仍走 REVIEW（弹人工），由 Hard Deny / 策略层负责硬拒绝。
func TestDecisionFromJevosHighHarmGoesReviewNotAutoDeny(t *testing.T) {
	jr := JEVOSResult{
		Destructive: 0.95, DataLoss: 0.9, NeedsReview: 0.3, RiskScore: 0.95,
	}
	if DecisionFromJevos(jr) != DecisionReview {
		t.Fatalf("want REVIEW not auto DENY, got %s", DecisionFromJevos(jr))
	}
}

func TestDecisionFromJevosLowRiskAllow(t *testing.T) {
	jr := JEVOSResult{
		Destructive: 0.1, DataLoss: 0.1, ServiceImpact: 0.1,
		PermissionRisk: 0.1, CredentialRisk: 0.1, NeedsReview: 0.2,
		RiskScore: 0.2,
	}
	if DecisionFromJevos(jr) != DecisionAllow {
		t.Fatalf("want ALLOW, got %s", DecisionFromJevos(jr))
	}
}

func TestJSONStateIncludesAgentContext(t *testing.T) {
	a := FromShell("claude", 1, 2, "npm test", "/ws", "")
	a.RulesPrompt = "允许在工作区内跑测试"
	a.PolicyNote = "allow_write=true"
	a.ContextSnippet = "用户: 帮我跑一下单测"
	st := a.JSONState()
	if st["agent_rules"] != a.RulesPrompt {
		t.Fatalf("missing agent_rules: %+v", st)
	}
	if st["agent_policy"] != a.PolicyNote {
		t.Fatalf("missing agent_policy: %+v", st)
	}
	if st["conversation_context"] != a.ContextSnippet {
		t.Fatalf("missing conversation_context: %+v", st)
	}
}

func TestGatewayJevosErrorGoesReview(t *testing.T) {
	g := NewGateway(NewMemorySessions(), jevosStub{err: true})
	a := FromShell("codex", 1, 1, "docker restart mysql", "/opt/project", "production")
	d := g.Evaluate(nil, a, EvaluateOpts{})
	if d.Decision != DecisionReview || d.DecidedBy != DecidedByJevos {
		t.Fatalf("got %+v", d)
	}
}

func TestGatewayJevosAllow(t *testing.T) {
	g := NewGateway(NewMemorySessions(), jevosStub{score: 0.2})
	a := FromShell("codex", 1, 1, "docker restart mysql", "/opt/project", "production")
	a.Purpose = "Restart MySQL after configuration change"
	d := g.Evaluate(nil, a, EvaluateOpts{})
	if d.Decision != DecisionAllow || d.DecidedBy != DecidedByJevos {
		t.Fatalf("got %+v", d)
	}
}

// 高 riskScore（含 needs_review）不得再自动 DENY，应转人工 REVIEW。
func TestGatewayJevosHighScoreGoesReview(t *testing.T) {
	g := NewGateway(NewMemorySessions(), jevosStub{score: 0.85})
	a := FromShell("claude", 1, 1, "npm test", "/ws", "")
	a.RulesPrompt = "允许运行测试"
	d := g.Evaluate(nil, a, EvaluateOpts{})
	if d.Decision != DecisionReview || d.DecidedBy != DecidedByJevos {
		t.Fatalf("want REVIEW, got %+v", d)
	}
}

func TestFromClaudeMapsActionType(t *testing.T) {
	r := FromClaude("claude", 1, 2, "Read", map[string]any{"file_path": "/ws/a.go"}, "/ws")
	if r.ActionType != ActionFileRead {
		t.Fatalf("got %s", r.ActionType)
	}
	w := FromClaude("claude", 1, 2, "Write", map[string]any{"file_path": "/ws/a.go"}, "/ws")
	if w.ActionType != ActionFileWrite {
		t.Fatalf("got %s", w.ActionType)
	}
}

type jevosStub struct {
	score float64
	err   bool
}

func (j jevosStub) Review(_ context.Context, a ToolAction) (JEVOSResult, error) {
	if j.err {
		return JEVOSResult{}, errJevosDown
	}
	return JEVOSResult{RiskScore: j.score, NeedsReview: j.score}, nil
}
