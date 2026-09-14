package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// GenericAsk 用任意本机 CLI 跑一轮问答(兼容无 session 的调用方)。
func GenericAsk(ctx context.Context, bin, dir, systemPrompt, question, history string, onChunk func(string), timeout time.Duration, onActivity ActivityFn) (string, int, error) {
	return GenericAskMeta(ctx, bin, dir, systemPrompt, question, history, onChunk, timeout, onActivity, nil, "")
}

// GenericAskMeta 同 GenericAsk，支持 Cursor --resume 与 session/usage 回调。
// history 保留参数以兼容旧调用；v0.2.18 起调用方应传空，由引擎 resume 维护上下文。
func GenericAskMeta(ctx context.Context, bin, dir, systemPrompt, question, history string, onChunk func(string), timeout time.Duration, onActivity ActivityFn, onMeta MetaFn, resumeSession string) (string, int, error) {
	if timeout <= 0 {
		timeout = 300 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	userInput := strings.TrimSpace(history)
	if userInput != "" {
		userInput += "\n\n"
	}
	userInput += question
	prompt := userInput
	if systemPrompt != "" {
		prompt = "【系统规则】\n" + systemPrompt + "\n\n【用户】\n" + userInput
	}

	if onActivity != nil {
		name := filepath.Base(bin)
		if name == "" {
			name = "agent"
		}
		onActivity(Activity{Tool: name, Summary: name + " 执行中…"})
	}

	start := time.Now()
	// 优先 JSON：便于解析 session_id（Cursor Agent）
	argsJSON := []string{"-p", "--output-format", "json"}
	if rs := strings.TrimSpace(resumeSession); rs != "" {
		argsJSON = append(argsJSON, "--resume", rs)
	}
	argsJSON = append(argsJSON, prompt)

	out, err := runOnce(cctx, bin, dir, argsJSON)
	if err != nil {
		// 回退：纯文本 -p / 无 -p
		argsText := []string{"-p"}
		if rs := strings.TrimSpace(resumeSession); rs != "" {
			argsText = append(argsText, "--resume", rs)
		}
		argsText = append(argsText, prompt)
		out2, err2 := runOnce(cctx, bin, dir, argsText)
		if err2 != nil {
			out3, err3 := runOnce(cctx, bin, dir, []string{prompt})
			if err3 != nil {
				return "", int(time.Since(start).Milliseconds()), fmt.Errorf("%v; fallback: %v; %v", err, err2, err3)
			}
			out = out3
		} else {
			out = out2
		}
	}

	text, meta := ParseCursorPrintOutput(out)
	if onMeta != nil && (meta.SessionID != "" || meta.InputTokens > 0 || meta.OutputTokens > 0) {
		onMeta(meta)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		text = strings.TrimSpace(out)
	}
	if onChunk != nil && text != "" {
		onChunk(text)
	}
	return text, int(time.Since(start).Milliseconds()), nil
}

// ParseCursorPrintOutput 解析 agent -p --output-format json 的结果行；非 JSON 则全文当正文。
func ParseCursorPrintOutput(raw string) (text string, meta RunMeta) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", meta
	}
	// 可能多行：取最后一段像 JSON 的对象
	lines := strings.Split(raw, "\n")
	var lastObj string
	for i := len(lines) - 1; i >= 0; i-- {
		l := strings.TrimSpace(lines[i])
		if strings.HasPrefix(l, "{") && strings.HasSuffix(l, "}") {
			lastObj = l
			break
		}
	}
	if lastObj == "" && strings.HasPrefix(raw, "{") {
		lastObj = raw
	}
	if lastObj == "" {
		return raw, meta
	}
	var m map[string]interface{}
	if json.Unmarshal([]byte(lastObj), &m) != nil {
		return raw, meta
	}
	if sid, _ := m["session_id"].(string); sid != "" {
		meta.SessionID = sid
	}
	if r, ok := m["result"].(string); ok {
		text = r
	}
	if u, _ := m["usage"].(map[string]interface{}); u != nil {
		meta.InputTokens = jsonInt64(u["inputTokens"])
		if meta.InputTokens == 0 {
			meta.InputTokens = jsonInt64(u["input_tokens"])
		}
		meta.OutputTokens = jsonInt64(u["outputTokens"])
		if meta.OutputTokens == 0 {
			meta.OutputTokens = jsonInt64(u["output_tokens"])
		}
		meta.CacheRead = jsonInt64(u["cacheReadTokens"])
		if meta.CacheRead == 0 {
			meta.CacheRead = jsonInt64(u["cache_read_input_tokens"])
		}
		meta.CacheWrite = jsonInt64(u["cacheWriteTokens"])
	}
	meta.UsedTokens = meta.InputTokens + meta.CacheRead
	if text == "" {
		text = raw
	}
	return text, meta
}

func runOnce(ctx context.Context, bin, dir string, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = CleanEnv(os.Environ())
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%s: %s", bin, msg)
	}
	s := stdout.String()
	if strings.TrimSpace(s) == "" {
		s = stderr.String()
	}
	return s, nil
}
