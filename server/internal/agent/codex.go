package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// CodexCommandEvent Codex 命令执行事件（用于时间线落库）。
type CodexCommandEvent struct {
	Command  string
	Output   string
	ExitCode int
	Done     bool
}

// CodexCommandFn 命令回调。
type CodexCommandFn func(CodexCommandEvent)

// CodexEvent 一行 JSONL 解析结果。
type CodexEvent struct {
	Activity *Activity
	Chunk    string
	Meta     *RunMeta
	Command  *CodexCommandEvent
}

// CodexArgs 构造非交互 `codex exec` 参数(禁止 -p:那是 profile)。
func CodexArgs(dir, prompt string, jsonMode bool) []string {
	args := []string{
		"exec",
		"--skip-git-repo-check",
		"-s", "workspace-write",
	}
	if jsonMode {
		args = append(args, "--json")
	}
	if strings.TrimSpace(dir) != "" {
		args = append(args, "-C", dir)
	}
	args = append(args, prompt)
	return args
}

// ParseCodexJSONLLine 解析 codex exec --json 的一行。
func ParseCodexJSONLLine(line []byte) (CodexEvent, bool) {
	var raw map[string]interface{}
	if json.Unmarshal(line, &raw) != nil {
		return CodexEvent{}, false
	}
	typ, _ := raw["type"].(string)
	ev := CodexEvent{}
	ok := false

	switch typ {
	case "thread.started":
		sid, _ := raw["thread_id"].(string)
		if sid != "" {
			ev.Meta = &RunMeta{SessionID: sid}
			ok = true
		}
	case "turn.started":
		ev.Activity = &Activity{Tool: "codex", Summary: "Codex 回合开始…"}
		ok = true
	case "item.started", "item.completed":
		item, _ := raw["item"].(map[string]interface{})
		if item == nil {
			return ev, false
		}
		it, _ := item["type"].(string)
		switch it {
		case "command_execution":
			cmd := strAny(item["command"])
			status := strAny(item["status"])
			out := strings.TrimSpace(strAny(item["aggregated_output"]))
			exit := 0
			if v, ok := item["exit_code"]; ok && v != nil {
				exit = int(jsonInt64(v))
			}
			done := typ == "item.completed"
			ev.Command = &CodexCommandEvent{Command: cmd, Output: out, ExitCode: exit, Done: done}
			summary := "执行命令 · " + truncateRunes(cmd, 72)
			if done {
				summary = "命令完成 · " + truncateRunes(cmd, 56)
				if out != "" {
					summary += " → " + truncateRunes(strings.ReplaceAll(out, "\n", " "), 40)
				}
			} else if status != "" && status != "in_progress" {
				summary = "命令 " + status + " · " + truncateRunes(cmd, 56)
			}
			ev.Activity = &Activity{Tool: "Bash", Summary: summary}
			ok = true
		case "agent_message":
			text := strAny(item["text"])
			if text != "" && typ == "item.completed" {
				// 时间线：每条消息以 · 分隔，便于阅读
				ev.Chunk = text
				ok = true
			}
		case "file_change", "patch", "apply_patch":
			ev.Activity = &Activity{Tool: "Edit", Summary: "正在编辑…"}
			ok = true
		case "mcp_tool_call", "tool_call", "function_call":
			name := strAny(item["name"])
			if name == "" {
				name = strAny(item["tool"])
			}
			if name == "" {
				name = "Tool"
			}
			phase := "调用"
			if typ == "item.completed" {
				phase = "完成"
			}
			ev.Activity = &Activity{Tool: name, Summary: phase + " " + name + "…"}
			ok = true
		case "error":
			msg := strAny(item["message"])
			if msg != "" {
				ev.Activity = &Activity{Tool: "codex", Summary: "提示 · " + truncateRunes(msg, 72)}
				ok = true
			}
		case "reasoning", "thought":
			// 不刷屏，仅轻量状态
			if typ == "item.started" {
				ev.Activity = &Activity{Tool: "codex", Summary: "思考中…"}
				ok = true
			}
		}
	case "turn.completed":
		usage, _ := raw["usage"].(map[string]interface{})
		if usage != nil {
			m := RunMeta{
				InputTokens:  jsonInt64(usage["input_tokens"]),
				OutputTokens: jsonInt64(usage["output_tokens"]),
				CacheRead:    jsonInt64(usage["cached_input_tokens"]),
				CacheWrite:   jsonInt64(usage["cache_write_input_tokens"]),
			}
			m.UsedTokens = m.InputTokens
			if m.UsedTokens == 0 {
				m.UsedTokens = m.InputTokens + m.CacheRead
			}
			ev.Meta = &m
			ok = true
		}
	}
	return ev, ok
}

func strAny(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	default:
		return ""
	}
}

// CodexAsk 用本机 Codex CLI 非交互跑一轮问答（优先 --json 流式）。
func CodexAsk(ctx context.Context, bin, dir, systemPrompt, question, history string, onChunk func(string), timeout time.Duration, onActivity ActivityFn) (string, int, error) {
	return CodexAskMeta(ctx, bin, dir, systemPrompt, question, history, onChunk, timeout, onActivity, nil, "", nil)
}

// CodexAskMeta 同 CodexAsk，额外支持用量/会话回调、resume 与命令时间线。
func CodexAskMeta(ctx context.Context, bin, dir, systemPrompt, question, history string, onChunk func(string), timeout time.Duration, onActivity ActivityFn, onMeta MetaFn, resumeThread string, onCommand CodexCommandFn) (string, int, error) {
	return CodexAskMetaEx(ctx, bin, dir, systemPrompt, question, history, onChunk, timeout, onActivity, onMeta, resumeThread, onCommand, nil)
}

// CodexAskMetaEx 增加 onCompact：JSONL 出现压缩事件时回调（用于平台清库同步）。
func CodexAskMetaEx(ctx context.Context, bin, dir, systemPrompt, question, history string, onChunk func(string), timeout time.Duration, onActivity ActivityFn, onMeta MetaFn, resumeThread string, onCommand CodexCommandFn, onCompact func()) (string, int, error) {
	if timeout <= 0 {
		timeout = 300 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	userInput := history
	if userInput != "" {
		userInput += "\n\n"
	}
	userInput += question
	prompt := userInput
	if systemPrompt != "" {
		prompt = "【系统规则】\n" + systemPrompt + "\n\n【用户】\n" + userInput
	}

	if bin == "" {
		bin = "codex"
	}
	start := time.Now()

	// resume：codex exec --json … resume <thread_id> <prompt>
	var args []string
	if rs := strings.TrimSpace(resumeThread); rs != "" {
		args = []string{
			"exec", "--json", "--skip-git-repo-check", "-s", "workspace-write",
			"--dangerously-bypass-hook-trust",
			"resume", rs, prompt,
		}
		if strings.TrimSpace(dir) != "" {
			// -C 需在 resume 子命令前
			args = []string{
				"exec", "--json", "--skip-git-repo-check", "-s", "workspace-write",
				"--dangerously-bypass-hook-trust",
				"-C", dir, "resume", rs, prompt,
			}
		}
	} else {
		args = CodexArgs(dir, prompt, true)
	}

	text, meta, err := runCodexJSONStream(cctx, bin, dir, args, onChunk, onActivity, onMeta, onCommand, onCompact)
	if err == nil {
		return text, int(time.Since(start).Milliseconds()), nil
	}
	// 回退：无 --json 的旧模式
	if onActivity != nil {
		onActivity(Activity{Tool: "codex", Summary: "Codex 执行中…"})
	}
	fallbackArgs := CodexArgs(dir, prompt, false)
	cmd := exec.CommandContext(cctx, bin, fallbackArgs...)
	AttachKillable(cmd)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = CleanEnv(os.Environ())
	cmd.Stdin = bytes.NewReader(nil)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	out := strings.TrimSpace(stdout.String())
	if out == "" {
		out = strings.TrimSpace(stderr.String())
	}
	if runErr != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = runErr.Error()
		}
		if text != "" {
			out = text
		}
		return out, int(time.Since(start).Milliseconds()), fmt.Errorf("codex: %s", msg)
	}
	if onChunk != nil && out != "" {
		onChunk(out)
	}
	_ = meta
	return out, int(time.Since(start).Milliseconds()), nil
}

func runCodexJSONStream(
	ctx context.Context, bin, dir string, args []string,
	onChunk func(string), onActivity ActivityFn, onMeta MetaFn, onCommand CodexCommandFn, onCompact func(),
) (string, RunMeta, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	AttachKillable(cmd)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = CleanEnv(os.Environ())
	// 避免 CLI 再从 stdin 读「Reading additional input」
	cmd.Stdin = bytes.NewReader(nil)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", RunMeta{}, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return "", RunMeta{}, err
	}

	var full strings.Builder
	var lastMeta RunMeta
	lastAct := ""
	sawJSON := false
	msgCount := 0
	compacted := false
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		if IsCodexCompactionLine(line) {
			sawJSON = true
			compacted = true
			// 延后到流结束后再 Sync，避免压缩事件夹在 agent_message 之后把 UI/库清掉
		}
		ev, ok := ParseCodexJSONLLine(line)
		if !ok {
			continue
		}
		sawJSON = true
		if ev.Meta != nil {
			if ev.Meta.SessionID != "" {
				lastMeta.SessionID = ev.Meta.SessionID
			}
			if ev.Meta.InputTokens > 0 || ev.Meta.OutputTokens > 0 {
				lastMeta.InputTokens = ev.Meta.InputTokens
				lastMeta.OutputTokens = ev.Meta.OutputTokens
				lastMeta.CacheRead = ev.Meta.CacheRead
				lastMeta.CacheWrite = ev.Meta.CacheWrite
				lastMeta.UsedTokens = ev.Meta.UsedTokens
			}
			if onMeta != nil {
				onMeta(lastMeta)
			}
		}
		if ev.Command != nil && onCommand != nil {
			onCommand(*ev.Command)
		}
		if ev.Activity != nil && onActivity != nil {
			key := ev.Activity.Tool + "|" + ev.Activity.Summary
			if key != lastAct {
				lastAct = key
				onActivity(*ev.Activity)
			}
		}
		if ev.Chunk != "" {
			// 时间线：每条 agent 消息前加换行 + ·
			piece := ev.Chunk
			if msgCount == 0 {
				piece = "· " + strings.TrimLeft(piece, "· ")
			} else {
				piece = "\n· " + strings.TrimLeft(piece, "· \n")
			}
			msgCount++
			full.WriteString(piece)
			if onChunk != nil {
				onChunk(piece)
			}
		}
	}
	_ = sc.Err()
	waitErr := cmd.Wait()
	if !sawJSON {
		return "", lastMeta, fmt.Errorf("codex: no json events: %s", strings.TrimSpace(stderr.String()))
	}
	if waitErr != nil && full.Len() == 0 {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = waitErr.Error()
		}
		return full.String(), lastMeta, fmt.Errorf("codex: %s", msg)
	}
	if onMeta != nil && (lastMeta.SessionID != "" || lastMeta.InputTokens > 0) {
		onMeta(lastMeta)
	}
	if compacted && onCompact != nil {
		onCompact()
	}
	return full.String(), lastMeta, nil
}
