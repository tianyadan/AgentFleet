package service

import (
	"testing"

	"colleague-avatar/server/internal/store"
)

func TestLatestTurnToPreserve(t *testing.T) {
	msgs := []store.Message{
		{Role: "assistant", Content: CompactNoticeText},
		{Role: "user", Content: "旧问题"},
		{Role: "assistant", Content: "旧回答"},
		{Role: "command", Content: "ls"},
		{Role: "user", Content: "新问题"},
		{Role: "command", Content: "pwd"},
		{Role: "assistant", Content: "新回答可见"},
	}
	u, a := LatestTurnToPreserve(msgs)
	if u != "新问题" || a != "新回答可见" {
		t.Fatalf("got user=%q assistant=%q", u, a)
	}
}

func TestLatestTurnToPreserveUserOnly(t *testing.T) {
	msgs := []store.Message{
		{Role: "user", Content: "进行中"},
		{Role: "command", Content: "echo"},
	}
	u, a := LatestTurnToPreserve(msgs)
	if u != "进行中" || a != "" {
		t.Fatalf("got user=%q assistant=%q", u, a)
	}
}

func TestCoalesceChatLogKeepsLongerLocalAssistant(t *testing.T) {
	// 纯函数在 frontend；此处用 Go 侧同名语义校验 preserve 逻辑不被 notice 覆盖
	msgs := []store.Message{
		{Role: "assistant", Content: CompactNoticeText},
		{Role: "user", Content: "q"},
	}
	u, a := LatestTurnToPreserve(msgs)
	if u != "q" || a != "" {
		t.Fatalf("notice should not count as assistant turn: u=%q a=%q", u, a)
	}
}
