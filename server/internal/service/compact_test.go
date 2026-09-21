package service

import "testing"

func TestCompactNoticeText(t *testing.T) {
	if CompactNoticeText == "" {
		t.Fatal("empty notice")
	}
	if !containsAll(CompactNoticeText, "引擎压缩", "会话已保留") {
		t.Fatalf("unexpected notice: %s", CompactNoticeText)
	}
}

func TestReinjectPrefix(t *testing.T) {
	p := ReinjectionPrefix()
	if p == "" || !containsAll(p, "压缩后续聊") {
		t.Fatalf("bad prefix: %q", p)
	}
}

func TestApplySystemReinject(t *testing.T) {
	base := "你是数字员工A"
	got := ApplySystemReinject(base, true)
	if !containsAll(got, "压缩后续聊", base) {
		t.Fatalf("want reinject+base, got %q", got)
	}
	same := ApplySystemReinject(base, false)
	if same != base {
		t.Fatalf("flag off should keep base, got %q", same)
	}
}

func TestIsCompactHookEvent(t *testing.T) {
	cases := map[string]bool{
		"PreCompact":  true,
		"PostCompact": true,
		"preCompact":  true,
		"PreToolUse":  false,
		"":            false,
	}
	for name, want := range cases {
		if got := IsCompactHookEvent(name); got != want {
			t.Fatalf("%s: got %v want %v", name, got, want)
		}
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !containsStr(s, p) {
			return false
		}
	}
	return true
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(func() bool {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		})())
}
