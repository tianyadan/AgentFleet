package agent

import "testing"

func TestIsCodexResumeGone(t *testing.T) {
	cases := []struct {
		msg  string
		want bool
	}{
		{"codex: Error: thread/resume: thread/resume failed: no rollout found for thread id 9dcd0f7f-41da-48a0-a035-76f88d654059 (code -32600)", true},
		{"no rollout found for thread id abc", true},
		{"thread/resume failed", true},
		{"permission denied", false},
		{"", false},
	}
	for _, c := range cases {
		if got := IsCodexResumeGone(c.msg); got != c.want {
			t.Fatalf("IsCodexResumeGone(%q)=%v want %v", c.msg, got, c.want)
		}
	}
}
