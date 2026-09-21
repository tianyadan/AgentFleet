package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// CompactArgs 构造各引擎 headless 原生压缩参数（不执行）。
// prompt 通常为 "/compact"。
func CompactArgs(engine, sessionID, prompt string) []string {
	sid := strings.TrimSpace(sessionID)
	p := strings.TrimSpace(prompt)
	if p == "" {
		p = "/compact"
	}
	switch strings.ToLower(strings.TrimSpace(engine)) {
	case "codex":
		return []string{
			"exec", "--skip-git-repo-check", "-s", "workspace-write",
			"resume", sid, p,
		}
	case "cursor", "agent":
		return []string{"-p", "--output-format", "json", "--resume", sid, p}
	default: // claude
		return []string{"-p", "--resume", sid, p}
	}
}

// RunEngineCompact 对已有 session 执行引擎原生压缩，保留同一 session。
func RunEngineCompact(ctx context.Context, bin, engine, dir, sessionID string, timeout time.Duration) error {
	sid := strings.TrimSpace(sessionID)
	if sid == "" {
		return fmt.Errorf("empty engine session")
	}
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if bin == "" {
		bin = DefaultBin(engine)
	}
	args := CompactArgs(engine, sid, "/compact")
	// Codex 需要 -C 在 resume 前
	if strings.EqualFold(strings.TrimSpace(engine), "codex") && strings.TrimSpace(dir) != "" {
		args = []string{
			"exec", "--skip-git-repo-check", "-s", "workspace-write",
			"-C", dir, "resume", sid, "/compact",
		}
	}
	cmd := exec.CommandContext(cctx, bin, args...)
	AttachKillable(cmd)
	if dir != "" && !strings.EqualFold(strings.TrimSpace(engine), "codex") {
		cmd.Dir = dir
	}
	cmd.Env = CleanEnv(os.Environ())
	cmd.Stdin = bytes.NewReader(nil)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s compact: %s", engine, msg)
	}
	return nil
}

// IsCodexCompactionLine 判断 Codex JSONL 是否为上下文压缩「完成」事件。
// 仅 completed（或顶层 compacted）触发，避免 item.started + Hook 重复清库把刚落库的回复清掉。
func IsCodexCompactionLine(line []byte) bool {
	var raw map[string]interface{}
	if json.Unmarshal(line, &raw) != nil {
		return false
	}
	typ := strings.ToLower(strings.TrimSpace(strAny(raw["type"])))
	if typ == "context_compacted" || typ == "thread.compacted" || strings.HasSuffix(typ, ".compacted") {
		return true
	}
	if typ != "item.completed" {
		return false
	}
	item, _ := raw["item"].(map[string]interface{})
	if item == nil {
		return false
	}
	it := strings.ToLower(strings.TrimSpace(strAny(item["type"])))
	switch it {
	case "context_compaction", "contextcompaction", "compaction":
		return true
	default:
		return strings.Contains(it, "compaction")
	}
}

// DefaultBin 按引擎名回落到常见可执行文件名（store 包也有同名逻辑时由调用方传入 bin）。
func DefaultBin(engine string) string {
	switch strings.ToLower(strings.TrimSpace(engine)) {
	case "codex":
		return "codex"
	case "cursor", "agent":
		return "agent"
	default:
		return "claude"
	}
}
