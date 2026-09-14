package store

import "testing"

func TestCommandAuditFields(t *testing.T) {
	a := CommandAudit{
		ConversationID: 1,
		AgentID:        2,
		ToolName:       "Bash",
		CommandText:    "ls",
		Decision:       "deny",
		DecidedBy:      "user",
		Risk:           "high",
	}
	if a.Decision != "deny" || a.DecidedBy != "user" {
		t.Fatalf("%+v", a)
	}
}
