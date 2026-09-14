package agent

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// GenericAsk 用任意本机 CLI 跑一轮问答(codex/agent 等)。
func GenericAsk(ctx context.Context, bin, dir, systemPrompt, question, history string, onChunk func(string), timeout time.Duration, onActivity ActivityFn) (string, int, error) {
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

	if onActivity != nil {
		name := filepath.Base(bin)
		if name == "" {
			name = "agent"
		}
		onActivity(Activity{Tool: name, Summary: name + " 执行中…"})
	}

	start := time.Now()
	out, err := runOnce(cctx, bin, dir, []string{"-p", prompt})
	if err != nil {
		out2, err2 := runOnce(cctx, bin, dir, []string{prompt})
		if err2 != nil {
			return "", int(time.Since(start).Milliseconds()), fmt.Errorf("%v; fallback: %v", err, err2)
		}
		out = out2
	}
	text := strings.TrimSpace(out)
	if onChunk != nil && text != "" {
		onChunk(text)
	}
	return text, int(time.Since(start).Milliseconds()), nil
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
