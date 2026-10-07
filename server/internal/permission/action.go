package permission

import (
	"path/filepath"
	"strings"
)

const (
	ActionFileRead         = "FILE_READ"
	ActionFileWrite        = "FILE_WRITE"
	ActionFileDelete       = "FILE_DELETE"
	ActionShellRead        = "SHELL_READ"
	ActionShellExec        = "SHELL_EXEC"
	ActionNetworkRead      = "NETWORK_READ"
	ActionNetworkWrite     = "NETWORK_WRITE"
	ActionProcessStart     = "PROCESS_START"
	ActionProcessStop      = "PROCESS_STOP"
	ActionServiceRestart   = "SERVICE_RESTART"
	ActionDBRead           = "DB_READ"
	ActionDBWrite          = "DB_WRITE"
	ActionDBDelete         = "DB_DELETE"
	ActionPermissionChange = "PERMISSION_CHANGE"
	ActionSystemConfig     = "SYSTEM_CONFIG"
)

const (
	DecisionAllow  = "ALLOW"
	DecisionReview = "REVIEW"
	DecisionDeny   = "DENY"
)

const (
	DecidedByStatic  = "static_rule"
	DecidedBySession = "session"
	DecidedByJevos   = "jevos"
	DecidedByHuman   = "human"
)

// ToolAction 平台统一工具动作（引擎 Adapter 产出，Gateway 消费）。
type ToolAction struct {
	Engine         string
	ConversationID int64
	AgentID        int64
	ActionType     string
	ToolName       string
	Command        string
	Args           map[string]any
	WorkingDir     string
	Environment    string
	Purpose        string
	PreExec        bool // 是否在执行前拦截；false 表示只能事后审计
	Raw            any
	argv           []string // 解析后的规范化 argv
}

// Signature 会话授权匹配键：精确规范化命令，禁止把 npm 当成万能授权。
func (a ToolAction) Signature() string {
	cmd := strings.Join(a.argv, " ")
	if cmd == "" {
		cmd = NormalizeCommand(a.Command)
	}
	return strings.Join([]string{
		a.ActionType,
		cmd,
		filepath.Clean(strings.TrimSpace(a.WorkingDir)),
		strings.TrimSpace(a.Environment),
		resourceScope(a),
	}, "\x1f")
}

func resourceScope(a ToolAction) string {
	if p, _ := a.Args["path"].(string); strings.TrimSpace(p) != "" {
		return strings.TrimSpace(p)
	}
	if p, _ := a.Args["file_path"].(string); strings.TrimSpace(p) != "" {
		return strings.TrimSpace(p)
	}
	return ""
}

// SideEffecting 静态规则无法确定时，有副作用操作才进 JEVOS。
func (a ToolAction) SideEffecting() bool {
	switch a.ActionType {
	case ActionFileRead, ActionShellRead, ActionDBRead, ActionNetworkRead:
		return false
	default:
		return true
	}
}

// FromClaude 把 Claude PreToolUse 转为 ToolAction。
func FromClaude(engine string, convID, agentID int64, tool string, input map[string]any, cwd string) ToolAction {
	a := ToolAction{
		Engine: engine, ConversationID: convID, AgentID: agentID,
		ToolName: tool, Args: input, WorkingDir: cwd, PreExec: true, Raw: input,
	}
	switch tool {
	case "Read", "Grep", "Glob", "LS", "NotebookRead":
		a.ActionType = ActionFileRead
		a.Command = briefPath(input)
	case "Write", "Edit", "MultiEdit", "NotebookEdit":
		a.ActionType = ActionFileWrite
		a.Command = briefPath(input)
	case "WebFetch", "WebSearch":
		a.ActionType = ActionNetworkRead
		a.Command = strAny(input["url"])
	case "Browser":
		a.ActionType = ActionNetworkWrite
		a.Command = tool
	case "Bash":
		a.Command = strAny(input["command"])
		return finishShellAction(a)
	default:
		a.ActionType = ActionShellExec
		a.Command = tool
	}
	return a
}

// FromShell Codex/Cursor 命令事件或 Bash 字符串。
func FromShell(engine string, convID, agentID int64, command, cwd, env string) ToolAction {
	a := ToolAction{
		Engine: engine, ConversationID: convID, AgentID: agentID,
		ToolName: "Bash", Command: command, WorkingDir: cwd, Environment: env,
		Args: map[string]any{"command": command},
	}
	return finishShellAction(a)
}

func finishShellAction(a ToolAction) ToolAction {
	a.argv = ParseArgv(a.Command)
	a.ActionType = ClassifyShellAction(a.argv, a.Command)
	if a.Args == nil {
		a.Args = map[string]any{}
	}
	a.Args["command"] = a.Command
	return a
}

func strAny(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

// JSONState JEVOS state 载荷。
func (a ToolAction) JSONState() map[string]any {
	return map[string]any{
		"engine":      a.Engine,
		"command":     a.Command,
		"working_dir": a.WorkingDir,
		"environment": a.Environment,
		"purpose":     a.Purpose,
		"action_type": a.ActionType,
	}
}
