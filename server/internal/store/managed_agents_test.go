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

func TestValidContextWindow(t *testing.T) {
	for _, n := range []int64{262144, 524288, 1000000} {
		if !ValidContextWindow(n) { t.Fatalf("expected supported window %d", n) }
	}
	for _, n := range []int64{0, 200000, 123456, -1} {
		if ValidContextWindow(n) { t.Fatalf("unexpected supported window %d", n) }
	}
}
