package service

import (
	"strings"
	"testing"
	"unicode"
)

func TestSafeProjectDirName(t *testing.T) {
	got := SafeProjectDirName("我的项目 / v1")
	if got == "" || strings.Contains(got, "/") {
		t.Fatalf("bad name: %q", got)
	}
	if SafeProjectDirName("") == "" {
		t.Fatal("empty should fallback")
	}
}

func TestTruncateRunesMax(t *testing.T) {
	s := strings.Repeat("字", 400)
	out := TruncateRunes(s, 300)
	if n := countRunes(out); n > 300 {
		t.Fatalf("got %d runes", n)
	}
}

func countRunes(s string) int {
	n := 0
	for range s {
		n++
	}
	_ = unicode.ReplacementChar
	return n
}
