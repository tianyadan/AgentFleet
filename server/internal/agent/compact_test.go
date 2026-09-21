package agent

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSettingsIncludesCompactHooks(t *testing.T) {
	s := HookSpec{Bin: "/tmp/avatar-hook", TimeoutS: 10}.Settings()
	if s == "" {
		t.Fatal("empty settings")
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatal(err)
	}
	hooks, ok := m["hooks"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing hooks: %s", s)
	}
	for _, name := range []string{"PreToolUse", "PreCompact", "PostCompact"} {
		if _, ok := hooks[name]; !ok {
			t.Fatalf("missing %s in %s", name, s)
		}
	}
	if !strings.Contains(s, "/tmp/avatar-hook") {
		t.Fatal("hook bin missing")
	}
}

func TestCompactArgsClaude(t *testing.T) {
	args := CompactArgs("claude", "sess-1", "/compact")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--resume") || !strings.Contains(joined, "sess-1") {
		t.Fatalf("want resume sess: %v", args)
	}
	if !strings.Contains(joined, "/compact") {
		t.Fatalf("want /compact: %v", args)
	}
	if !strings.Contains(joined, "-p") {
		t.Fatalf("want -p: %v", args)
	}
}

func TestCompactArgsCursor(t *testing.T) {
	args := CompactArgs("cursor", "chat-9", "/compact")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--resume") || !strings.Contains(joined, "chat-9") {
		t.Fatalf("want resume: %v", args)
	}
	if !strings.Contains(joined, "/compact") {
		t.Fatalf("want /compact: %v", args)
	}
}

func TestCompactArgsCodex(t *testing.T) {
	args := CompactArgs("codex", "thread-1", "/compact")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "resume") || !strings.Contains(joined, "thread-1") {
		t.Fatalf("want resume thread: %v", args)
	}
	if !strings.Contains(joined, "/compact") {
		t.Fatalf("want /compact: %v", args)
	}
}

func TestIsCodexCompactionEvent(t *testing.T) {
	line := []byte(`{"type":"item.completed","item":{"type":"context_compaction","status":"completed"}}`)
	if !IsCodexCompactionLine(line) {
		t.Fatal("want compaction true")
	}
	// camelCase 兼容
	lineCamel := []byte(`{"type":"item.completed","item":{"type":"contextCompaction","status":"completed"}}`)
	if !IsCodexCompactionLine(lineCamel) {
		t.Fatal("want camelCase compaction true")
	}
	// started 不触发，避免与 completed / Hook 重复清库
	lineStarted := []byte(`{"type":"item.started","item":{"type":"context_compaction"}}`)
	if IsCodexCompactionLine(lineStarted) {
		t.Fatal("started should not sync")
	}
	line2 := []byte(`{"type":"item.completed","item":{"type":"agent_message"}}`)
	if IsCodexCompactionLine(line2) {
		t.Fatal("want false")
	}
}
