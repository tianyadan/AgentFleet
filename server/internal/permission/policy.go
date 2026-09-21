package permission

import (
	"path/filepath"
	"strings"
)

// AgentPolicy 数字员工硬性权限(写入/联网/rm/工作区)。
type AgentPolicy struct {
	AllowWrite    bool
	AllowNetwork  bool
	AllowRm       bool
	WorkspacePath string // 非空则禁止跨区
}

// ApplyAgentPolicy 在 Classify 之后施加数字员工策略;命中则 Deny。
// 已开启联网时:WebFetch/WebSearch 直接 Allow,避免「网络策略拦截」式误拒。
func ApplyAgentPolicy(tool string, input map[string]interface{}, dec Decision, p AgentPolicy) Decision {
	if dec.Behavior == Deny {
		return dec
	}
	cmd := ""
	if tool == "Bash" {
		cmd = strings.ToLower(str(input, "command"))
	}

	// 联网类工具:策略优先于 Classify 的 Ask
	if tool == "WebFetch" || tool == "WebSearch" || tool == "Browser" {
		if p.AllowNetwork {
			return Decision{Allow, "数字员工已授权联网"}
		}
		return Decision{Deny, "数字员工策略禁止联网"}
	}

	if !p.AllowRm && looksLikeRm(tool, cmd) {
		return Decision{Deny, "数字员工策略禁止 rm 操作"}
	}
	if !p.AllowWrite && isWriteTool(tool, cmd, input) {
		return Decision{Deny, "数字员工策略禁止写入"}
	}
	if !p.AllowNetwork && isNetworkTool(tool, cmd) {
		return Decision{Deny, "数字员工策略禁止联网"}
	}
	if ws := strings.TrimSpace(p.WorkspacePath); ws != "" {
		if path := toolPath(tool, input); path != "" && !underRoot(path, ws) {
			return Decision{Deny, "越出绑定工作区:" + path}
		}
		if tool == "Bash" && cmdHasPathOutside(cmd, ws) {
			return Decision{Deny, "命令路径越出绑定工作区"}
		}
	}
	return dec
}

func looksLikeRm(tool, cmd string) bool {
	if tool != "Bash" {
		return false
	}
	fields := strings.Fields(cmd)
	for _, f := range fields {
		if f == "rm" || strings.HasSuffix(f, "/rm") {
			return true
		}
	}
	return strings.Contains(cmd, "rm -") || strings.Contains(cmd, "unlink ")
}

// LooksLikeRm 判断工具调用是否像 rm/unlink（编排「允许全部命令」仍须弹窗）。
func LooksLikeRm(tool string, input map[string]interface{}) bool {
	cmd := ""
	if tool == "Bash" {
		cmd = strings.ToLower(str(input, "command"))
	}
	return looksLikeRm(tool, cmd)
}

func isWriteTool(tool, cmd string, input map[string]interface{}) bool {
	if writeTools[tool] {
		return true
	}
	if tool != "Bash" {
		return false
	}
	if strings.ContainsAny(cmd, ">") {
		return true
	}
	for _, w := range []string{"mv ", "cp ", "tee ", "sed -i", "chmod ", "chown ", "mkdir ", "touch ", "install ", "npm install", "pip install", "git commit", "git push", "git add"} {
		if strings.Contains(cmd, w) {
			return true
		}
	}
	_ = input
	return false
}

func isNetworkTool(tool, cmd string) bool {
	if tool == "WebFetch" || tool == "WebSearch" || tool == "Browser" {
		return true
	}
	if tool != "Bash" {
		return false
	}
	for _, w := range []string{"curl ", "wget ", "http://", "https://", "npm publish", "pip install"} {
		if strings.Contains(cmd, w) {
			// 本机 API 例外仍由上层 Classify 处理;策略层一律视为联网
			return true
		}
	}
	return false
}

func toolPath(tool string, input map[string]interface{}) string {
	for _, k := range []string{"file_path", "path", "directory"} {
		if p := str(input, k); p != "" {
			return p
		}
	}
	_ = tool
	return ""
}

func underRoot(path, root string) bool {
	absP, err1 := filepath.Abs(path)
	absR, err2 := filepath.Abs(root)
	if err1 != nil || err2 != nil {
		return strings.HasPrefix(path, root)
	}
	rel, err := filepath.Rel(absR, absP)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// cmdHasPathOutside 粗检绝对路径是否越界(保守:发现 / 开头且不在 root 下则拦)。
func cmdHasPathOutside(cmd, root string) bool {
	absR, err := filepath.Abs(root)
	if err != nil {
		absR = root
	}
	for _, f := range strings.Fields(cmd) {
		if !strings.HasPrefix(f, "/") {
			continue
		}
		if !underRoot(f, absR) {
			return true
		}
	}
	return false
}
