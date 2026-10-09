package agent

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ErrCodexSessionContextUnavailable 表示本机 Codex rollout 中没有可用上下文快照。
// 调用方不得把累计用量或 turn usage 当作该快照的替代品。
var ErrCodexSessionContextUnavailable = errors.New("codex session context unavailable")

var codexThreadIDPattern = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z_-]{7,511}$`)

// CodexSessionContext 是 Codex 写入 rollout JSONL 的最新 token_count 观测。
// UsedTokens 是当前上下文，TotalTokens 是整个 session 的累计用量，二者绝不混用。
type CodexSessionContext struct {
	SessionID    string
	UsedTokens   int64
	WindowTokens int64
	TotalTokens  int64
	ObservedAt   time.Time
	Source       string
	Anomaly      string
}

// CodexSessionContextCollector 从本机 Codex sessions 目录读取单个 thread 的 rollout。
// SessionsRoot 可在测试中注入；空值时使用当前运行用户的 ~/.codex/sessions。
type CodexSessionContextCollector struct {
	SessionsRoot string
}

func NewCodexSessionContextCollector(sessionsRoot string) CodexSessionContextCollector {
	return CodexSessionContextCollector{SessionsRoot: sessionsRoot}
}

func DefaultCodexSessionContextCollector() CodexSessionContextCollector {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return CodexSessionContextCollector{}
	}
	return NewCodexSessionContextCollector(filepath.Join(home, ".codex", "sessions"))
}

// Collect 返回严格绑定 session_meta.payload.id 的最后一个有效 token_count。
// 只扫描与 thread ID 后缀匹配的日期目录，避免遍历其他用户的 rollout；不记录文件正文。
func (c CodexSessionContextCollector) Collect(sessionID string) (CodexSessionContext, error) {
	sessionID = strings.TrimSpace(sessionID)
	if !codexThreadIDPattern.MatchString(sessionID) || strings.TrimSpace(c.SessionsRoot) == "" {
		return CodexSessionContext{}, ErrCodexSessionContextUnavailable
	}
	pattern := filepath.Join(c.SessionsRoot, "*", "*", "*", "rollout-*-"+sessionID+".jsonl")
	paths, err := filepath.Glob(pattern)
	if err != nil || len(paths) == 0 {
		return CodexSessionContext{}, ErrCodexSessionContextUnavailable
	}
	sort.Strings(paths)
	var latest CodexSessionContext
	latestPath := ""
	found := false
	for _, path := range paths {
		snap, ok := collectCodexSessionFile(path, sessionID)
		if !ok {
			continue
		}
		if !found || snap.ObservedAt.After(latest.ObservedAt) || (snap.ObservedAt.Equal(latest.ObservedAt) && path > latestPath) {
			latest, latestPath, found = snap, path, true
		}
	}
	if !found {
		return CodexSessionContext{}, ErrCodexSessionContextUnavailable
	}
	return latest, nil
}

func collectCodexSessionFile(path, sessionID string) (CodexSessionContext, bool) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return CodexSessionContext{}, false
	}
	f, err := os.Open(path)
	if err != nil {
		return CodexSessionContext{}, false
	}
	defer f.Close()

	var last CodexSessionContext
	validMeta := false
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		var event codexSessionEvent
		if json.Unmarshal(sc.Bytes(), &event) != nil {
			continue
		}
		if event.Type == "session_meta" && event.Payload.ID == sessionID {
			validMeta = true
			continue
		}
		if event.Payload.Type != "token_count" || event.Payload.Info.LastTokenUsage == nil || event.Payload.Info.LastTokenUsage.TotalTokens < 0 || event.Payload.Info.ModelContextWindow <= 0 {
			continue
		}
		last = CodexSessionContext{
			SessionID:    sessionID,
			UsedTokens:   event.Payload.Info.LastTokenUsage.TotalTokens,
			WindowTokens: event.Payload.Info.ModelContextWindow,
			Source:       "codex_session",
			ObservedAt:   parseCodexEventTime(event.Timestamp),
		}
		if event.Payload.Info.TotalTokenUsage != nil && event.Payload.Info.TotalTokenUsage.TotalTokens >= 0 {
			last.TotalTokens = event.Payload.Info.TotalTokenUsage.TotalTokens
		}
		if last.ObservedAt.IsZero() {
			last.ObservedAt = info.ModTime()
		}
		if last.UsedTokens > last.WindowTokens {
			last.Anomaly = fmt.Sprintf("current_context_exceeds_window: used=%d window=%d", last.UsedTokens, last.WindowTokens)
		}
	}
	if sc.Err() != nil || !validMeta || last.Source == "" {
		return CodexSessionContext{}, false
	}
	return last, true
}

type codexSessionEvent struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Payload   struct {
		ID   string `json:"id"`
		Type string `json:"type"`
		Info struct {
			LastTokenUsage     *codexTokenUsage `json:"last_token_usage"`
			TotalTokenUsage    *codexTokenUsage `json:"total_token_usage"`
			ModelContextWindow int64            `json:"model_context_window"`
		} `json:"info"`
	} `json:"payload"`
}

type codexTokenUsage struct {
	TotalTokens int64 `json:"total_tokens"`
}

func parseCodexEventTime(raw string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t
		}
	}
	return time.Time{}
}
