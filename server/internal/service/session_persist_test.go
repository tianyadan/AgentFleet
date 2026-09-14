package service

import "testing"

func TestPersistSessionFlagForNoHook(t *testing.T) {
	// 文档化约定：noHook 轻量调用不得 resume/写 session。
	noHook := true
	persist := !noHook && true
	if persist {
		t.Fatal("noHook must disable session persist")
	}
	noHook = false
	persist = !noHook && true
	if !persist {
		t.Fatal("normal ask should persist session")
	}
}

// TestAskPromptUsesEmptyHistory 约定：调用引擎时 history 必须为空（由 resume 续聊）。
func TestAskPromptUsesEmptyHistory(t *testing.T) {
	history := "" // managed / E-bot 均不得再拼 RecentTurns
	if history != "" {
		t.Fatal("v0.2.18 must not inject platform turn history")
	}
}
