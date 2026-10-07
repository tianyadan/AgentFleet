package visitorcookie

import "testing"

func TestSignParseRoundTrip(t *testing.T) {
	raw := Sign("secret", "abc123")
	pid, ok := Parse("secret", raw)
	if !ok || pid != "abc123" {
		t.Fatalf("got %q ok=%v", pid, ok)
	}
	if _, ok := Parse("wrong", raw); ok {
		t.Fatal("bad secret should fail")
	}
}
