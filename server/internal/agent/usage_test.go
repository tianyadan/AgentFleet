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
