package config

import "testing"

func TestContextWindowForEngine(t *testing.T) {
	c := Config{
		ContextWindowClaude: 100000,
		ContextWindowCodex:  200000,
		ContextWindowCursor: 128000,
	}
	if c.ContextWindowForEngine("codex") != 200000 {
		t.Fatalf("codex")
	}
	if c.ContextWindowForEngine("agent") != 128000 {
		t.Fatalf("cursor/agent")
	}
	if c.ContextWindowForEngine("claude") != 100000 {
		t.Fatalf("claude")
	}
	if (Config{}).ContextWindowForEngine("codex") != 200000 {
		t.Fatalf("codex empty fallback")
	}
	if (Config{}).ContextWindowForEngine("claude") != 1000000 {
		t.Fatalf("claude empty fallback")
	}
}

func TestClaudeDefaultContextWindow(t *testing.T) {
	t.Setenv("AVATAR_CONTEXT_WINDOW_CLAUDE", "")
	if got := Load().ContextWindowForEngine("claude"); got != 1000000 {
		t.Fatalf("Claude default window = %d, want 1000000", got)
	}
	t.Setenv("AVATAR_CONTEXT_WINDOW_CLAUDE", "200000")
	if got := Load().ContextWindowForEngine("claude"); got != 200000 {
		t.Fatalf("Claude env override = %d, want 200000", got)
	}
}

func TestOSSConfigured(t *testing.T) {
	c := Config{}
	if c.OSSConfigured() {
		t.Fatal("empty should be false")
	}
	c = Config{
		OSSEndpoint: "oss-cn-qingdao.aliyuncs.com",
		OSSAccessKeyID: "id", OSSAccessKeySecret: "sec",
		OSSBucket: "digital-employee-qd",
		OSSPublicBase: "https://digital-employee-qd.oss-cn-qingdao.aliyuncs.com",
	}
	if !c.OSSConfigured() {
		t.Fatal("want configured")
	}
}

func TestNormalizeOSSPublicBaseRewritesBrokenCNAME(t *testing.T) {
	got := normalizeOSSPublicBase("https://digital-employee-qd.cn-qingdao.taihangcda.cn/")
	if got != officialOSSPublicBase {
		t.Fatalf("got %q", got)
	}
	keep := "https://digital-employee-qd.oss-cn-qingdao.aliyuncs.com"
	if normalizeOSSPublicBase(keep) != keep {
		t.Fatal("official base should stay")
	}
}
