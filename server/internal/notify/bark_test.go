package notify

import "testing"

func TestNotifyDedupe(t *testing.T) {
	n := New("echo", true)
	n.runner = func(name string, args ...string) error { return nil }

	if !n.PermissionOnce("req-1", "cmd ls") {
		t.Fatal("first should send")
	}
	if n.PermissionOnce("req-1", "cmd ls") {
		t.Fatal("same id must not resend")
	}
	if !n.PermissionOnce("req-2", "cmd pwd") {
		t.Fatal("new id should send")
	}
}

func TestNotifyDisabled(t *testing.T) {
	n := New("echo", false)
	called := false
	n.runner = func(name string, args ...string) error {
		called = true
		return nil
	}
	if n.PermissionOnce("x", "y") {
		t.Fatal("disabled must not send")
	}
	if called {
		t.Fatal("runner should not run")
	}
}
