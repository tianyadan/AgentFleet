package agent

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseCodexJSONLCommandAndMessage(t *testing.T) {
	lines := []string{
		`{"type":"thread.started","thread_id":"tid-1"}`,
		`{"type":"item.started","item":{"id":"item_1","type":"command_execution","command":"/bin/zsh -lc pwd","status":"in_progress"}}`,
		`{"type":"item.completed","item":{"id":"item_1","type":"command_execution","command":"/bin/zsh -lc pwd","aggregated_output":"/tmp\n","exit_code":0,"status":"completed"}}`,
		`{"type":"item.completed","item":{"id":"item_2","type":"agent_message","text":"done"}}`,
		`{"type":"turn.completed","usage":{"input_tokens":100,"cached_input_tokens":10,"output_tokens":5}}`,
	}
	var acts []Activity
	var chunks []string
	var metas []RunMeta
	for _, line := range lines {
		ev, ok := ParseCodexJSONLLine([]byte(line))
		if !ok {
			t.Fatalf("parse fail: %s", line)
		}
		if ev.Activity != nil {
			acts = append(acts, *ev.Activity)
		}
		if ev.Chunk != "" {
			chunks = append(chunks, ev.Chunk)
		}
		if ev.Meta != nil {
			metas = append(metas, *ev.Meta)
		}
	}
	if len(acts) < 2 || !strings.Contains(acts[0].Summary, "pwd") {
		t.Fatalf("acts=%v", acts)
	}
	// Chunk 原文仍是 done；时间线前缀在 runCodexJSONStream 层添加
	if strings.Join(chunks, "") != "done" {
		t.Fatalf("chunks=%v", chunks)
	}
	if len(metas) == 0 || metas[len(metas)-1].SessionID != "tid-1" && metas[0].SessionID != "tid-1" {
		// session may arrive on first meta; usage on last
		foundSid := false
		for _, m := range metas {
			if m.SessionID == "tid-1" {
				foundSid = true
			}
		}
		if !foundSid {
			t.Fatalf("metas=%v", metas)
		}
	}
	var usage RunMeta
	for _, m := range metas {
		if m.InputTokens > 0 {
			usage = m
		}
	}
	if usage.InputTokens != 100 || usage.OutputTokens != 5 {
		t.Fatalf("usage=%+v", usage)
	}
}

func TestParseCodexJSONLInvalid(t *testing.T) {
	_, ok := ParseCodexJSONLLine([]byte("not-json"))
	if ok {
		t.Fatal("expected false")
	}
	_ = json.RawMessage{}
}
