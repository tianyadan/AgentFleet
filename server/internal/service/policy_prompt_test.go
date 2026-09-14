package service

import (
	"strings"
	"testing"

	"colleague-avatar/server/internal/store"
)

func TestAgentPolicyBlockAndReinforce(t *testing.T) {
	a := &store.ManagedAgent{
		Name: "x", AllowWrite: false, AllowNetwork: false, AllowRm: false,
		WorkspacePath: "/tmp/ws",
	}
	block := AgentPolicyBlock(a)
	if !strings.Contains(block, "禁止") || !strings.Contains(block, "/tmp/ws") {
		t.Fatalf("bad block: %s", block)
	}
	q := MaybeReinforcePolicy(a, 10, "hello")
	if !strings.Contains(q, "策略重申") || !strings.Contains(q, "hello") {
		t.Fatalf("bad reinforce: %s", q)
	}
	if MaybeReinforcePolicy(a, 9, "hello") != "hello" {
		t.Fatal("should not reinforce on turn 9")
	}
}

func TestAgentPolicyBlockBrowserCloseHint(t *testing.T) {
	a := &store.ManagedAgent{Name: "b", AllowBrowser: true}
	block := AgentPolicyBlock(a)
	if !strings.Contains(block, "及时关闭网页") || !strings.Contains(block, "僵尸网页") {
		t.Fatalf("missing browser close hint: %s", block)
	}
}
