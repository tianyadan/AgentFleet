package agent

import "testing"

func TestParseClaudeContextText(t *testing.T) {
	used, win, ok := ParseClaudeContextText("**Tokens:** 24.1k / 1m (2%)")
	if !ok || used != 24100 || win != 1000000 {
		t.Fatalf("got used=%d win=%d ok=%v", used, win, ok)
	}
}

func TestParseClaudeStreamMetaResult(t *testing.T) {
	line := []byte(`{"type":"result","session_id":"abc","usage":{"input_tokens":100,"output_tokens":5,"cache_read_input_tokens":10},"modelUsage":{"m":{"contextWindow":200000,"inputTokens":100,"outputTokens":5}}}`)
	m, ok := ParseClaudeStreamMeta(line)
	if !ok || m.SessionID != "abc" || m.InputTokens != 100 || m.OutputTokens != 5 || m.ContextWindow != 200000 || m.UsedTokens != 110 {
		t.Fatalf("got %+v ok=%v", m, ok)
	}
}

func TestParseClaudeStreamMetaSessionIdCamel(t *testing.T) {
	line := []byte(`{"type":"system","sessionId":"sess-camel"}`)
	m, ok := ParseClaudeStreamMeta(line)
	if !ok || m.SessionID != "sess-camel" {
		t.Fatalf("got %+v ok=%v", m, ok)
	}
}

func TestExtractSessionIDPrefersSnake(t *testing.T) {
	raw := map[string]interface{}{"session_id": "a", "sessionId": "b", "thread_id": "c"}
	if got := ExtractSessionID(raw); got != "a" {
		t.Fatalf("got %q", got)
	}
}

func TestExtractSessionIDNestedThread(t *testing.T) {
	raw := map[string]interface{}{"type": "thread.started", "thread": map[string]interface{}{"id": "thr-nested"}}
	if got := ExtractSessionID(raw); got != "thr-nested" {
		t.Fatalf("got %q", got)
	}
}

func TestStickySessionIDKeepsFirst(t *testing.T) {
	if got := StickySessionID("parent", "child"); got != "parent" {
		t.Fatalf("got %q", got)
	}
	if got := StickySessionID("", " child "); got != "child" {
		t.Fatalf("got %q", got)
	}
}
