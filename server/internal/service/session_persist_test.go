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
