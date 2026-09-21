// avatar-hook 是引擎 CLI 的 hook 桥：
// 1) PreToolUse：把工具授权请求转发到数字分身后端，等真人前端裁决。
// 2) PreCompact / PostCompact / preCompact：通知后端同步清库（平台对话历史）。
//
// 由后端 --settings / 工作区 hooks.json 注入，输入为 stdin 上的 hook JSON。
package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type hookInput struct {
	HookEventName string                 `json:"hook_event_name"`
	ToolName      string                 `json:"tool_name"`
	ToolInput     map[string]interface{} `json:"tool_input"`
	SessionID     string                 `json:"session_id"`
	Cwd           string                 `json:"cwd"`
	Trigger       string                 `json:"trigger"`
}

type decideReq struct {
	ToolName  string                 `json:"tool_name"`
	ToolInput map[string]interface{} `json:"tool_input"`
	SessionID string                 `json:"session_id"`
	Cwd       string                 `json:"cwd"`
	ConvID    string                 `json:"conversation_id"`
	AgentID   string                 `json:"agent_id"`
}

type decideResp struct {
	Behavior string `json:"behavior"` // allow / deny
	Reason   string `json:"reason"`
}

func main() {
	raw, _ := io.ReadAll(os.Stdin)
	var in hookInput
	_ = json.Unmarshal(raw, &in)

	// Cursor 等可能用不同字段名
	if in.HookEventName == "" {
		var alt map[string]interface{}
		if json.Unmarshal(raw, &alt) == nil {
			if v, ok := alt["hook_event_name"].(string); ok {
				in.HookEventName = v
			} else if v, ok := alt["hookEventName"].(string); ok {
				in.HookEventName = v
			}
		}
	}

	if isCompactEvent(in.HookEventName) {
		notifyCompact(in)
		emitEmpty()
		return
	}

	// 无工具名:不表态,交回 CLI 默认策略。
	if in.ToolName == "" {
		emitEmpty()
		return
	}

	base := os.Getenv("AVATAR_BACKEND_URL")
	if base == "" {
		base = "http://127.0.0.1:8080"
	}
	client := &http.Client{Timeout: 5 * time.Minute}

	reqBody, _ := json.Marshal(decideReq{
		ToolName:  in.ToolName,
		ToolInput: in.ToolInput,
		SessionID: in.SessionID,
		Cwd:       in.Cwd,
		ConvID:    os.Getenv("AVATAR_CONV_ID"),
		AgentID:   os.Getenv("AVATAR_POLICY_AGENT_ID"),
	})
	req, err := http.NewRequest("POST", base+"/api/permissions/request", bytes.NewReader(reqBody))
	if err != nil {
		_ = json.NewEncoder(os.Stdout).Encode(buildHookOutput("deny", "授权桥接初始化失败", in.ToolName, nil))
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		_ = json.NewEncoder(os.Stdout).Encode(buildHookOutput("deny", "无法连接分身后端,已拒绝", in.ToolName, nil))
		return
	}
	defer resp.Body.Close()

	var d decideResp
	_ = json.NewDecoder(resp.Body).Decode(&d)
	if d.Behavior == "allow" {
		_ = json.NewEncoder(os.Stdout).Encode(buildHookOutput("allow", d.Reason, in.ToolName, in.ToolInput))
		return
	}
	reason := d.Reason
	if reason == "" {
		reason = "未获授权,已拒绝"
	}
	_ = json.NewEncoder(os.Stdout).Encode(buildHookOutput("deny", reason, in.ToolName, in.ToolInput))
}

func isCompactEvent(name string) bool {
	switch strings.TrimSpace(name) {
	case "PreCompact", "PostCompact", "preCompact":
		return true
	default:
		return false
	}
}

// notifyCompact 通知后端同步清库；失败不影响引擎压缩（仅打空响应）。
func notifyCompact(in hookInput) {
	base := os.Getenv("AVATAR_BACKEND_URL")
	if base == "" {
		base = "http://127.0.0.1:8080"
	}
	body, _ := json.Marshal(map[string]interface{}{
		"conversation_id": os.Getenv("AVATAR_CONV_ID"),
		"session_id":      in.SessionID,
		"trigger":         in.Trigger,
		"hook_event_name": in.HookEventName,
	})
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("POST", base+"/api/permissions/compact-sync", bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	_ = resp.Body.Close()
}

// buildHookOutput 按 PreToolUse 协议构造裁决。
// 用户/AI 明确放行 Bash 时写入 dangerouslyDisableSandbox,避免沙箱二次拦截已授权命令。
func buildHookOutput(behavior, reason, toolName string, toolInput map[string]interface{}) map[string]interface{} {
	hso := map[string]interface{}{
		"hookEventName":            "PreToolUse",
		"permissionDecision":       behavior,
		"permissionDecisionReason": reason,
	}
	if behavior == "allow" && toolName == "Bash" {
		updated := map[string]interface{}{}
		for k, v := range toolInput {
			updated[k] = v
		}
		updated["dangerouslyDisableSandbox"] = true
		hso["updatedInput"] = updated
	}
	return map[string]interface{}{"hookSpecificOutput": hso}
}

// emitEmpty 不表态,交给 CLI 自身权限逻辑。
func emitEmpty() {
	_ = json.NewEncoder(os.Stdout).Encode(map[string]interface{}{})
}
