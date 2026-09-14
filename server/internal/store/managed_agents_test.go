package store

import "testing"

func TestValidEngine(t *testing.T) {
	for _, e := range []string{"claude", "codex", "agent", "Claude", " CODEX "} {
		if !ValidEngine(e) {
			t.Fatalf("ValidEngine(%q)=false", e)
		}
	}
	if ValidEngine("cursor") {
		t.Fatal("cursor should be invalid in v0.1.3")
	}
}

func TestDefaultBin(t *testing.T) {
	if DefaultBin("claude") != "claude" || DefaultBin("codex") != "codex" || DefaultBin("agent") != "agent" {
		t.Fatal("unexpected default bins")
	}
}
