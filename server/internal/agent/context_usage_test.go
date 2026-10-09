package agent

import (
	"errors"
	"testing"
)

var errProbeFailed = errors.New("probe failed")

// turn usage ≠ 真实 context occupancy：Codex/Cursor 必须标估算。
func TestNormalizeRunMetaContext_CodexEstimated(t *testing.T) {
	m := RunMeta{InputTokens: 1000, CacheRead: 200, OutputTokens: 50}
	out := NormalizeRunMetaContext("codex", m, 200000)
	if !out.Estimated {
		t.Fatal("codex must be estimated")
	}
	if out.UsedTokens != 1200 {
		t.Fatalf("used=%d want input+cache=1200 (not cumulative across turns)", out.UsedTokens)
	}
	if out.ContextWindow != 200000 {
		t.Fatalf("window=%d", out.ContextWindow)
	}
}

func TestNormalizeRunMetaContext_CursorEstimated(t *testing.T) {
	m := RunMeta{InputTokens: 800, CacheRead: 0, ContextWindow: 0}
	out := NormalizeRunMetaContext("agent", m, 128000)
	if !out.Estimated || out.UsedTokens != 800 || out.ContextWindow != 128000 {
		t.Fatalf("got %+v", out)
	}
}

// Claude stream usage 也只是 turn 估算；真实值靠 /context probe。
func TestNormalizeRunMetaContext_ClaudeStreamEstimated(t *testing.T) {
	m := RunMeta{InputTokens: 500, CacheRead: 100, ContextWindow: 200000}
	out := NormalizeRunMetaContext("claude", m, 0)
	if !out.Estimated {
		t.Fatal("claude stream usage must be estimated until probe")
	}
	if out.UsedTokens != 600 || out.ContextWindow != 200000 {
		t.Fatalf("got %+v", out)
	}
}

func TestContextFromProbe_NotEstimated(t *testing.T) {
	s := ContextFromProbe("sess", 24100, 1000000)
	if s.Estimated || s.Source != "probe" || s.UsedTokens != 24100 || s.WindowTokens != 1000000 {
		t.Fatalf("got %+v", s)
	}
	if s.UsedPercent < 2.0 || s.UsedPercent > 2.5 {
		t.Fatalf("pct=%v", s.UsedPercent)
	}
}

func TestBuildEngineContextSnapshot_CodexDoesNotTreatTurnUsageAsContext(t *testing.T) {
	s := BuildEngineContextSnapshot(ContextBuildInput{
		Engine:         "codex",
		SessionID:      "t1",
		CachedUsed:     3000,
		CachedWindow:   0,
		FallbackWindow: 200000,
	})
	if !s.Estimated || s.Source != "unavailable" || s.WindowTokens != 0 || s.UsedTokens != 0 {
		t.Fatalf("got %+v", s)
	}
}

func TestContextFromCodexSessionPreservesRawAnomaly(t *testing.T) {
	s := ContextFromCodexSession(CodexSessionContext{
		SessionID: "thread", UsedTokens: 300, WindowTokens: 200, TotalTokens: 900,
		Source: "codex_session", Anomaly: "current_context_exceeds_window",
	})
	if s.Estimated || s.Source != "codex_session" || s.UsedPercent != 150 || s.TotalTokens != 900 || s.Anomaly == "" {
		t.Fatalf("got %+v", s)
	}
}

func TestBuildEngineContextSnapshot_ClaudeProbeSuccess(t *testing.T) {
	s := BuildEngineContextSnapshot(ContextBuildInput{
		Engine:         "claude",
		SessionID:      "c1",
		CachedUsed:     100,
		CachedWindow:   200000,
		FallbackWindow: 200000,
		AllowProbe:     true,
		Probe: func() (used, window int64, raw string, err error) {
			return 50000, 200000, "Tokens: 50k / 200k", nil
		},
	})
	if s.Estimated || s.Source != "probe" || s.UsedTokens != 50000 {
		t.Fatalf("got %+v", s)
	}
}

func TestBuildEngineContextSnapshot_ClaudeProbeFailFallback(t *testing.T) {
	s := BuildEngineContextSnapshot(ContextBuildInput{
		Engine:         "claude",
		SessionID:      "c1",
		CachedUsed:     900,
		CachedWindow:   200000,
		FallbackWindow: 200000,
		AllowProbe:     true,
		Probe: func() (used, window int64, raw string, err error) {
			return 0, 0, "", errProbeFailed
		},
	})
	if !s.Estimated || s.Source != "cached" || s.UsedTokens != 900 {
		t.Fatalf("got %+v", s)
	}
}

func TestInferEstimatedFromLegacy_NoField(t *testing.T) {
	if InferEstimated("claude", "probe") {
		t.Fatal("claude probe legacy => not estimated")
	}
	if !InferEstimated("claude", "cached") {
		t.Fatal("claude cached legacy => estimated")
	}
	if InferEstimated("codex", "codex_session") {
		t.Fatal("codex session snapshot => not estimated")
	}
	if !InferEstimated("codex", "cached") || !InferEstimated("agent", "") {
		t.Fatal("codex/cursor default estimated")
	}
}
