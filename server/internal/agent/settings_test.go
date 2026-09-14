package agent

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSettingsIncludesUnsandboxedEscape(t *testing.T) {
	s := HookSpec{Bin: "/tmp/avatar-hook", TimeoutS: 10}.Settings()
	if s == "" {
		t.Fatal("empty settings")
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatal(err)
	}
	sb, ok := m["sandbox"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing sandbox: %s", s)
	}
	if sb["allowUnsandboxedCommands"] != true {
		t.Fatalf("allowUnsandboxedCommands=%v", sb["allowUnsandboxedCommands"])
	}
	if !strings.Contains(s, "PreToolUse") {
		t.Fatal("hooks missing")
	}
}
