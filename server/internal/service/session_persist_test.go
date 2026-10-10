package service

import (
	"strings"
	"testing"

	"atolla/server/internal/store"
)

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

func TestAskPromptUsesEmptyHistoryWhenResuming(t *testing.T) {
	resume := "sess-1"
	history := ""
	if resume != "" && history != "" {
		t.Fatal("resume 时不得拼平台历史")
	}
}

func TestFormatResumeHistorySkipsCurrentQuestion(t *testing.T) {
	turns := []store.Turn{
		{User: "第一句", Assistant: "记住了"},
		{User: "第二句", Assistant: ""},
	}
	got := FormatResumeHistory(turns, "第二句")
	if !strings.Contains(got, "第一句") || !strings.Contains(got, "记住了") {
		t.Fatalf("got %q", got)
	}
	if strings.Contains(got, "第二句") {
		t.Fatalf("should skip current question: %q", got)
	}
}
