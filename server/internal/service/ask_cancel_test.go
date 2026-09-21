package service

import (
	"errors"
	"testing"
)

func TestApplyCancelledAskOutputKeepsPartial(t *testing.T) {
	out, err := applyCancelledAskOutput(errors.New("canceled"), "仓库已分析", errors.New("signal: killed"))
	if err != nil {
		t.Fatalf("runErr should be cleared, got %v", err)
	}
	if out != "仓库已分析\n\n⏹ 已终止本次对话" {
		t.Fatalf("got %q", out)
	}
}

func TestApplyCancelledAskOutputEmpty(t *testing.T) {
	out, err := applyCancelledAskOutput(errors.New("canceled"), "No response requested.", errors.New("killed"))
	if err != nil {
		t.Fatalf("got %v", err)
	}
	if out != "⏹ 已终止本次对话" {
		t.Fatalf("got %q", out)
	}
}

func TestApplyCancelledAskOutputKeepsSuccess(t *testing.T) {
	out, err := applyCancelledAskOutput(nil, "完成", nil)
	if err != nil || out != "完成" {
		t.Fatalf("got %q %v", out, err)
	}
}

func TestContextProbeBlockedWhileRunning(t *testing.T) {
	if !contextProbeBlocked("running", false) {
		t.Fatal("running should block /context probe")
	}
	if !contextProbeBlocked("idle", true) {
		t.Fatal("active run should block even before status flips")
	}
	if contextProbeBlocked("idle", false) {
		t.Fatal("idle should allow probe")
	}
}
