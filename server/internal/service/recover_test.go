package service

import "testing"

func TestIsTerminalKillErr(t *testing.T) {
	if !isTerminalKillErr(errString("agent: signal: killed")) {
		t.Fatal("want true for signal: killed")
	}
	if !isTerminalKillErr(errString("context deadline exceeded")) {
		t.Fatal("want true for deadline")
	}
	if isTerminalKillErr(errString("API key is invalid")) {
		t.Fatal("want false for api key")
	}
}

type errString string

func (e errString) Error() string { return string(e) }
