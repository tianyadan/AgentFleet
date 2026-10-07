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
		t.Fatalf("empty fallback")
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
		OSSPublicBase: "https://digital-employee-qd.cn-qingdao.taihangcda.cn",
	}
	if !c.OSSConfigured() {
		t.Fatal("want configured")
	}
}
