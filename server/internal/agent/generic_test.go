package agent

import "testing"

func TestParseCursorPrintOutput(t *testing.T) {
	raw := `{"type":"result","subtype":"success","result":"hello","session_id":"abc-123","usage":{"inputTokens":10,"outputTokens":2,"cacheReadTokens":1}}`
	text, meta := ParseCursorPrintOutput(raw)
	if text != "hello" {
		t.Fatalf("text=%q", text)
	}
	if meta.SessionID != "abc-123" {
		t.Fatalf("session=%q", meta.SessionID)
	}
	if meta.InputTokens != 10 || meta.OutputTokens != 2 || meta.CacheRead != 1 {
		t.Fatalf("meta=%+v", meta)
	}
}

func TestParseCursorPrintOutputPlain(t *testing.T) {
	text, meta := ParseCursorPrintOutput("just plain")
	if text != "just plain" || meta.SessionID != "" {
		t.Fatalf("text=%q meta=%+v", text, meta)
	}
}
