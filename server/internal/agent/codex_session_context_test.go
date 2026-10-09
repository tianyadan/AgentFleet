package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func writeCodexSession(t *testing.T, root, name, body string) {
	t.Helper()
	dir := filepath.Join(root, "2026", "10", "08")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCodexSessionContextCollectorUsesLastValidSnapshotForResumeAndCompaction(t *testing.T) {
	root := t.TempDir()
	threadID := "01a11111-1111-7111-8111-111111111111"
	writeCodexSession(t, root, "rollout-2026-10-08T10-00-00-"+threadID+".jsonl", `
{"type":"session_meta","payload":{"id":"01a11111-1111-7111-8111-111111111111"}}
{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"total_tokens":120000},"total_token_usage":{"total_tokens":250000},"model_context_window":258400}}}
{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"total_tokens":120000},"total_token_usage":{"total_tokens":250000},"model_context_window":258400}}}
{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"total_tokens":42000},"total_token_usage":{"total_tokens":310000},"model_context_window":258400}}}
`)

	snap, err := NewCodexSessionContextCollector(root).Collect(threadID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.UsedTokens != 42000 || snap.WindowTokens != 258400 || snap.TotalTokens != 310000 {
		t.Fatalf("got %+v, want last compacted snapshot", snap)
	}
	if snap.Source != "codex_session" || snap.ObservedAt.IsZero() {
		t.Fatalf("missing observation metadata: %+v", snap)
	}
}

func TestCodexSessionContextCollectorRejectsWrongMetaAndMarksAnomaly(t *testing.T) {
	root := t.TempDir()
	threadID := "01a22222-2222-7222-8222-222222222222"
	writeCodexSession(t, root, "rollout-2026-10-08T10-00-00-"+threadID+".jsonl", `
{"type":"session_meta","payload":{"id":"wrong-thread"}}
{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"total_tokens":1},"total_token_usage":{"total_tokens":2},"model_context_window":3}}}
`)
	writeCodexSession(t, root, "rollout-2026-10-08T11-00-00-"+threadID+".jsonl", `
{"type":"session_meta","payload":{"id":"01a22222-2222-7222-8222-222222222222"}}
{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"total_tokens":300},"total_token_usage":{"total_tokens":900},"model_context_window":200}}}
`)

	snap, err := NewCodexSessionContextCollector(root).Collect(threadID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.UsedTokens != 300 || snap.Anomaly == "" {
		t.Fatalf("expected preserved over-window observation, got %+v", snap)
	}
}

func TestCodexSessionContextCollectorTreatsMissingFieldsAndSessionsAsUnavailable(t *testing.T) {
	root := t.TempDir()
	threadID := "01a33333-3333-7333-8333-333333333333"
	writeCodexSession(t, root, "rollout-2026-10-08T10-00-00-"+threadID+".jsonl", `
{"type":"session_meta","payload":{"id":"01a33333-3333-7333-8333-333333333333"}}
{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"total_tokens":99}}}}
not-json
`)

	if _, err := NewCodexSessionContextCollector(root).Collect(threadID); err == nil {
		t.Fatal("missing last usage/window must be unavailable")
	}
	if _, err := NewCodexSessionContextCollector(root).Collect("01a44444-4444-7444-8444-444444444444"); err == nil {
		t.Fatal("missing session must be unavailable")
	}
}
