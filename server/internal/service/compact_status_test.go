package service

import "testing"

func TestStatusZhCompressing(t *testing.T) {
	// 与前端约定：compressing → 压缩记忆（后端常量校验）
	if AgentStatusCompressing != "compressing" {
		t.Fatalf("want compressing, got %s", AgentStatusCompressing)
	}
}

func TestCompactStatusTransition(t *testing.T) {
	// running 压缩后应回到 running；idle 压缩后回 idle
	if got := CompactStatusAfterSync("running"); got != "running" {
		t.Fatalf("got %s", got)
	}
	if got := CompactStatusAfterSync("compressing"); got != "idle" {
		t.Fatalf("got %s", got)
	}
	if got := CompactStatusAfterSync("idle"); got != "idle" {
		t.Fatalf("got %s", got)
	}
	if got := CompactStatusAfterSync("error"); got != "idle" {
		t.Fatalf("got %s", got)
	}
}
