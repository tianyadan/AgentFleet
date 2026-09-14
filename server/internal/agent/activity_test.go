package agent

import (
	"strings"
	"testing"
)

func TestSummarizeToolActivityBashShort(t *testing.T) {
	a := SummarizeToolActivity("Bash", map[string]interface{}{"command": "git status"})
	if a.Summary != "执行命令 · git status" {
		t.Fatalf("got %q", a.Summary)
	}
}

func TestSummarizeToolActivityEditNoPathNoise(t *testing.T) {
	a := SummarizeToolActivity("Edit", map[string]interface{}{"file_path": "/secret/a.go", "old_string": "x"})
	if strings.Contains(a.Summary, "/secret") || strings.Contains(a.Summary, "old_string") {
		t.Fatalf("should not leak path/diff: %s", a.Summary)
	}
	if a.Summary != "正在编辑…" {
		t.Fatalf("got %q", a.Summary)
	}
}

func TestParseStreamActivityToolUse(t *testing.T) {
	line := []byte(`{"type":"stream_event","event":{"type":"content_block_start","content_block":{"type":"tool_use","name":"Bash","input":{"command":"ls"}}}}`)
	a, ok := ParseStreamActivity(line)
	if !ok || a.Tool != "Bash" {
		t.Fatalf("ok=%v a=%+v", ok, a)
	}
}
