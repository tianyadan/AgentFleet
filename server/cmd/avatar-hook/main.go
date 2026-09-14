// avatar-hook 是 claude CLI 的 PreToolUse hook:把工具授权请求转发到数字分身后端,
// 等真人前端点「同意/拒绝」后,再把裁决按 hook 协议打印回 stdout。
//
// 由后端 --settings 注入调用,输入为 stdin 上的 PreToolUse JSON。
package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"time"
)

type hookInput struct {
	HookEventName string                 `json:"hook_event_name"`
	ToolName      string                 `json:"tool_name"`
	ToolInput     map[string]interface{} `json:"tool_input"`
	SessionID     string                 `json:"session_id"`
	Cwd           string                 `json:"cwd"`
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
