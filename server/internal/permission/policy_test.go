package permission

import "testing"

func TestApplyAgentPolicyAllowWebFetchWhenNetworkOn(t *testing.T) {
	p := AgentPolicy{AllowWrite: true, AllowNetwork: true, AllowRm: false}
	d := ApplyAgentPolicy("WebFetch", map[string]interface{}{"url": "https://x"}, Decision{Ask, "需确认"}, p)
	if d.Behavior != Allow {
		t.Fatalf("want Allow got %s %s", d.Behavior, d.Reason)
	}
}

func TestApplyAgentPolicyDenyWebFetchWhenNetworkOff(t *testing.T) {
	p := AgentPolicy{AllowWrite: true, AllowNetwork: false, AllowRm: true}
	d := ApplyAgentPolicy("WebFetch", map[string]interface{}{"url": "https://x"}, Decision{Ask, "需确认"}, p)
	if d.Behavior != Deny {
		t.Fatalf("want Deny got %s", d.Behavior)
	}
}

func TestApplyAgentPolicyDenyRm(t *testing.T) {
	p := AgentPolicy{AllowWrite: true, AllowNetwork: true, AllowRm: false}
	d := ApplyAgentPolicy("Bash", map[string]interface{}{"command": "rm -rf /tmp/x"}, Decision{Ask, ""}, p)
	if d.Behavior != Deny {
		t.Fatalf("want deny rm, got %v", d)
	}
}

func TestApplyAgentPolicyDenyWrite(t *testing.T) {
	p := AgentPolicy{AllowWrite: false, AllowNetwork: true, AllowRm: true}
	d := ApplyAgentPolicy("Write", map[string]interface{}{"file_path": "/ws/a.go"}, Decision{Ask, ""}, p)
	if d.Behavior != Deny {
		t.Fatalf("want deny write, got %v", d)
	}
}

func TestApplyAgentPolicyWorkspace(t *testing.T) {
	p := AgentPolicy{AllowWrite: true, AllowNetwork: true, AllowRm: true, WorkspacePath: "/Users/t/ws"}
	d := ApplyAgentPolicy("Read", map[string]interface{}{"file_path": "/etc/passwd"}, Decision{Allow, ""}, p)
	if d.Behavior != Deny {
		t.Fatalf("want deny outside ws, got %v", d)
	}
	d2 := ApplyAgentPolicy("Read", map[string]interface{}{"file_path": "/Users/t/ws/a.go"}, Decision{Allow, ""}, p)
	if d2.Behavior != Allow {
		t.Fatalf("want allow inside, got %v", d2)
	}
}
