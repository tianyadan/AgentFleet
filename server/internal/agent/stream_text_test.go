package agent

import "testing"

func TestClaudeLineTextDelta(t *testing.T) {
	line := []byte(`{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"你好"}}}`)
	text, snapshot, isResult := claudeLineText(line)
	if text != "你好" || snapshot || isResult {
		t.Fatalf("got %q snapshot=%v result=%v", text, snapshot, isResult)
	}
}

func TestClaudeLineTextAssistantSnapshot(t *testing.T) {
	line := []byte(`{"type":"assistant","message":{"content":[{"type":"thinking","thinking":"x"},{"type":"text","text":"抱歉，刚才漏了"}]}}`)
	text, snapshot, isResult := claudeLineText(line)
	if text != "抱歉，刚才漏了" || !snapshot || isResult {
		t.Fatalf("got %q snapshot=%v result=%v", text, snapshot, isResult)
	}
}

func TestClaudeLineTextResult(t *testing.T) {
	line := []byte(`{"type":"result","result":"最终回复"}`)
	text, snapshot, isResult := claudeLineText(line)
	if text != "最终回复" || !snapshot || !isResult {
		t.Fatalf("got %q snapshot=%v result=%v", text, snapshot, isResult)
	}
}

func TestClaudeLineTextSkipsToolOnly(t *testing.T) {
	line := []byte(`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash"}]}}`)
	text, _, _ := claudeLineText(line)
	if text != "" {
		t.Fatalf("tool_use should not be text, got %q", text)
	}
}
