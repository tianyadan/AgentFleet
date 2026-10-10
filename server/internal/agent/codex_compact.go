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

// Codex 手动压缩走 app-server；exec resume "/compact" 会被当成普通 prompt，可能挂很久。
const codexCompactTimeoutCap = 5 * time.Minute

// RunCodexCompact 通过 `codex app-server` 的 thread/compact/start 做原生压缩。
func RunCodexCompact(ctx context.Context, bin, dir, sessionID string, timeout time.Duration) error {
	sid := strings.TrimSpace(sessionID)
	if sid == "" {
		return fmt.Errorf("empty engine session")
	}
	if timeout <= 0 || timeout > codexCompactTimeoutCap {
		timeout = codexCompactTimeoutCap
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if strings.TrimSpace(bin) == "" {
		bin = DefaultBin("codex")
	}
	cmd := exec.CommandContext(cctx, bin, "app-server", "--stdio")
	AttachKillable(cmd)
	if strings.TrimSpace(dir) != "" {
		cmd.Dir = dir
	}
	cmd.Env = CleanEnv(os.Environ())
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("codex compact stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("codex compact stdout: %w", err)
	}
	var stderrBuf strings.Builder
	cmd.Stderr = &stderrBuf
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("codex app-server start: %w", err)
	}
	defer func() {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	enc := json.NewEncoder(stdin)
	enc.SetEscapeHTML(false)
	send := func(obj map[string]interface{}) error {
		if err := enc.Encode(obj); err != nil {
			return err
		}
		return nil
	}
	reader := bufio.NewReader(stdout)
	// 超时靠 CommandContext 杀进程 → stdout EOF，避免并发读 bufio.Reader。
	next := func() (map[string]interface{}, error) {
		return readCodexAppServerMessage(reader)
	}
	waitID := func(id int) (map[string]interface{}, error) {
		for {
			msg, err := next()
			if err != nil {
				return nil, err
			}
			if rpcIDEquals(msg["id"], id) {
				if errObj, ok := msg["error"].(map[string]interface{}); ok && errObj != nil {
					return nil, fmt.Errorf("%s", codexRPCErrorText(errObj))
				}
				return msg, nil
			}
			// 服务端可能反向发 request（审批等）；压缩路径不交互，直接拒绝以免挂死。
			if method, _ := msg["method"].(string); method != "" && msg["id"] != nil && msg["result"] == nil && msg["error"] == nil {
				_ = send(map[string]interface{}{
					"id": msg["id"],
					"error": map[string]interface{}{
						"code":    -32000,
						"message": "avatar compact: client cannot handle server request",
					},
				})
			}
		}
	}

	if err := send(map[string]interface{}{
		"method": "initialize",
		"id":     1,
		"params": map[string]interface{}{
			"clientInfo": map[string]interface{}{
				"name":    "atolla",
				"title":   "atolla",
				"version": "1.0.0",
			},
		},
	}); err != nil {
		return fmt.Errorf("codex compact initialize: %w", err)
	}
	if _, err := waitID(1); err != nil {
		return fmt.Errorf("codex compact initialize: %w", err)
	}
	if err := send(map[string]interface{}{"method": "initialized", "params": map[string]interface{}{}}); err != nil {
		return fmt.Errorf("codex compact initialized: %w", err)
	}

	resumeParams := map[string]interface{}{
		"threadId": sid,
	}
	if strings.TrimSpace(dir) != "" {
		resumeParams["cwd"] = dir
	}
	if err := send(map[string]interface{}{
		"method": "thread/resume",
		"id":     2,
		"params": resumeParams,
	}); err != nil {
		return fmt.Errorf("codex compact resume: %w", err)
	}
	if _, err := waitID(2); err != nil {
		return fmt.Errorf("codex compact resume: %w", err)
	}

	if err := send(map[string]interface{}{
		"method": "thread/compact/start",
		"id":     3,
		"params": map[string]interface{}{"threadId": sid},
	}); err != nil {
		return fmt.Errorf("codex compact start: %w", err)
	}
	if _, err := waitID(3); err != nil {
		return fmt.Errorf("codex compact start: %w", err)
	}

	for {
		msg, err := next()
		if err != nil {
			tail := strings.TrimSpace(stderrBuf.String())
			if tail != "" {
				return fmt.Errorf("codex compact wait: %v (%s)", err, truncateRunes(tail, 240))
			}
			return fmt.Errorf("codex compact wait: %w", err)
		}
		if IsCodexAppServerCompactionDone(msg) {
			return nil
		}
		if errMsg := codexAppServerTurnFailed(msg); errMsg != "" {
			return fmt.Errorf("codex compact: %s", errMsg)
		}
		if rpcIDEquals(msg["id"], 3) {
			if errObj, ok := msg["error"].(map[string]interface{}); ok && errObj != nil {
				return fmt.Errorf("codex compact start: %s", codexRPCErrorText(errObj))
			}
		}
		// 压缩过程中的服务端 request：拒绝，避免阻塞。
		if method, _ := msg["method"].(string); method != "" && msg["id"] != nil && msg["result"] == nil && msg["error"] == nil {
			_ = send(map[string]interface{}{
				"id": msg["id"],
				"error": map[string]interface{}{
					"code":    -32000,
					"message": "avatar compact: client cannot handle server request",
				},
			})
		}
	}
}

// IsCodexAppServerCompactionDone 判断 app-server JSON-RPC 通知是否为压缩完成。
func IsCodexAppServerCompactionDone(msg map[string]interface{}) bool {
	if msg == nil {
		return false
	}
	method := strings.ToLower(strings.TrimSpace(strAny(msg["method"])))
	params, _ := msg["params"].(map[string]interface{})
	if params == nil {
		params = map[string]interface{}{}
	}
	switch method {
	case "item/completed":
		item, _ := params["item"].(map[string]interface{})
		if item == nil {
			return false
		}
		it := strings.ToLower(strings.TrimSpace(strAny(item["type"])))
		return it == "contextcompaction" || it == "context_compaction" || strings.Contains(it, "compaction")
	case "thread/compacted", "context_compacted":
		return true
	default:
		if strings.HasSuffix(method, "/compacted") || strings.HasSuffix(method, ".compacted") {
			return true
		}
		return false
	}
}

func readCodexAppServerMessage(r *bufio.Reader) (map[string]interface{}, error) {
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			return nil, err
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var msg map[string]interface{}
		if err := json.Unmarshal(line, &msg); err != nil {
			return nil, fmt.Errorf("bad json: %w", err)
		}
		return msg, nil
	}
}

func rpcIDEquals(v interface{}, want int) bool {
	switch t := v.(type) {
	case float64:
		return int(t) == want
	case int:
		return t == want
	case int64:
		return int(t) == want
	case json.Number:
		n, err := t.Int64()
		return err == nil && int(n) == want
	default:
		return false
	}
}

func codexRPCErrorText(errObj map[string]interface{}) string {
	msg := strings.TrimSpace(strAny(errObj["message"]))
	if msg == "" {
		msg = fmt.Sprintf("%v", errObj)
	}
	return msg
}

// codexAppServerTurnFailed 从 turn/completed 提取失败原因（成功返回空）。
func codexAppServerTurnFailed(msg map[string]interface{}) string {
	if msg == nil {
		return ""
	}
	method := strings.ToLower(strings.TrimSpace(strAny(msg["method"])))
	if method != "turn/completed" {
		return ""
	}
	params, _ := msg["params"].(map[string]interface{})
	if params == nil {
		return ""
	}
	turn, _ := params["turn"].(map[string]interface{})
	if turn == nil {
		return ""
	}
	status := strings.ToLower(strings.TrimSpace(strAny(turn["status"])))
	if status == "" || status == "completed" || status == "success" || status == "ok" {
		return ""
	}
	if errObj, ok := turn["error"].(map[string]interface{}); ok && errObj != nil {
		if m := strings.TrimSpace(strAny(errObj["message"])); m != "" {
			return m
		}
	}
	if m := strings.TrimSpace(strAny(turn["error"])); m != "" {
		return m
	}
	return "turn status=" + status
}
