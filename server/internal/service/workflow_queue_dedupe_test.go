package service

import "testing"

func TestAppendUniqueQueueSkipsVisitedAndDupes(t *testing.T) {
	visited := map[string]bool{"a": true}
	q := []string{"x"}
	q = appendUniqueQueue(q, visited, "a", "b", "b", "x", "c")
	if len(q) != 3 || q[0] != "x" || q[1] != "b" || q[2] != "c" {
		t.Fatalf("got %#v", q)
	}
}

func TestDedupeReadyNodeIDs(t *testing.T) {
	ids := []string{"n1", "n2", "n1", "n3", "n2"}
	got := dedupeStrings(ids)
	if len(got) != 3 || got[0] != "n1" || got[1] != "n2" || got[2] != "n3" {
		t.Fatalf("got %#v", got)
	}
}
