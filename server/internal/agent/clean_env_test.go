package agent

import (
	"strings"
	"testing"
)

func TestCleanEnvStripsCursorAPIKey(t *testing.T) {
	in := []string{
		"PATH=/usr/bin",
		"CURSOR_API_KEY=crsr_bad",
		"CURSOR_AGENT=1",
		"HOME=/Users/x",
	}
	out := CleanEnv(in)
	joined := strings.Join(out, "\n")
	if strings.Contains(joined, "CURSOR_API_KEY") || strings.Contains(joined, "CURSOR_AGENT=") {
		t.Fatalf("cursor env should be stripped: %v", out)
	}
	if !strings.Contains(joined, "PATH=") || !strings.Contains(joined, "HOME=") {
		t.Fatalf("kept env lost: %v", out)
	}
}
