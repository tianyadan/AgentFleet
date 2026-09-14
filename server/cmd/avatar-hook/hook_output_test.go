package main

import (
	"encoding/json"
	"testing"
)

func TestBuildAllowBashDisablesSandbox(t *testing.T) {
	out := buildHookOutput("allow", "ok", "Bash", map[string]interface{}{
		"command": "ls -la",
	})
	hso, _ := out["hookSpecificOutput"].(map[string]interface{})
	if hso["permissionDecision"] != "allow" {
		t.Fatalf("decision=%v", hso["permissionDecision"])
	}
	upd, ok := hso["updatedInput"].(map[string]interface{})
	if !ok {
		t.Fatal("missing updatedInput")
	}
	if upd["command"] != "ls -la" {
		t.Fatalf("command lost: %v", upd["command"])
	}
	if upd["dangerouslyDisableSandbox"] != true {
		t.Fatalf("sandbox flag=%v", upd["dangerouslyDisableSandbox"])
	}
}

func TestBuildAllowNonBashNoUpdatedInput(t *testing.T) {
	out := buildHookOutput("allow", "", "Read", map[string]interface{}{"file_path": "/tmp/a"})
	hso := out["hookSpecificOutput"].(map[string]interface{})
	if _, ok := hso["updatedInput"]; ok {
		t.Fatal("Read should not rewrite input")
	}
}

func TestBuildDenyNoUpdatedInput(t *testing.T) {
	out := buildHookOutput("deny", "no", "Bash", map[string]interface{}{"command": "rm -rf /"})
	raw, _ := json.Marshal(out)
	var parsed map[string]interface{}
	_ = json.Unmarshal(raw, &parsed)
	hso := parsed["hookSpecificOutput"].(map[string]interface{})
	if _, ok := hso["updatedInput"]; ok {
		t.Fatal("deny must not set updatedInput")
	}
}
