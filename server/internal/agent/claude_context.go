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

// ClaudeProbeContext 对已有 session 执行 /context，解析真实窗口占用。
func ClaudeProbeContext(ctx context.Context, bin, dir, sessionID string, timeout time.Duration) (used, window int64, raw string, err error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return 0, 0, "", fmt.Errorf("no engine session")
	}
	if bin == "" {
		bin = "claude"
	}
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := []string{"-p", "--resume", sessionID, "--output-format", "stream-json", "--verbose", "/context"}
	cmd := exec.CommandContext(cctx, bin, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = CleanEnv(os.Environ())
	cmd.Stdin = bytes.NewReader(nil)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return 0, 0, "", err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return 0, 0, "", err
	}
	var resultText strings.Builder
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		var rawMap map[string]interface{}
		if json.Unmarshal(line, &rawMap) != nil {
			continue
		}
		if typ, _ := rawMap["type"].(string); typ == "result" {
			if r, _ := rawMap["result"].(string); r != "" {
				resultText.WriteString(r)
			}
		}
	}
	_ = cmd.Wait()
	raw = resultText.String()
	if raw == "" {
		raw = stderr.String()
	}
	used, window, ok := ParseClaudeContextText(raw)
	if !ok {
		return 0, 0, raw, fmt.Errorf("未能解析 /context 输出")
	}
	return used, window, raw, nil
}
