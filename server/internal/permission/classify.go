// Package permission 把 claude CLI 的工具授权请求(PreToolUse hook)代理到前端,
// 由真人点「同意/拒绝」后回传给 hook。只读操作自动放行,写/危险/越权才打扰用户。
package permission

import (
	"path/filepath"
	"strings"
)

// Behavior 授权裁决结果。
type Behavior string

const (
	Allow Behavior = "allow" // 自动放行,不弹窗
	Ask   Behavior = "ask"   // 需用户在前端确认
	Deny  Behavior = "deny"  // 自动拒绝(仅灾难性命令)
)

// Decision 一次分类结果。
type Decision struct {
	Behavior Behavior `json:"behavior"`
	Reason   string   `json:"reason"`
}

// 只读文件类工具:命中授权目录即放行。
var readOnlyTools = map[string]bool{
	"Read":         true,
	"Grep":         true,
	"Glob":         true,
	"LS":           true,
	"NotebookRead": true,
}

// 写类工具:交给用户判断(弹窗),不再静默拒绝。
var writeTools = map[string]bool{
	"Write":        true,
	"Edit":         true,
	"MultiEdit":    true,
	"NotebookEdit": true,
}

// 只读 shell 命令白名单(首词)。
var readOnlyCmds = map[string]bool{
	"cat": true, "head": true, "tail": true, "less": true, "more": true,
	"ls": true, "tree": true, "find": true, "du": true, "wc": true, "stat": true,
	"grep": true, "rg": true, "ag": true, "awk": true, "sed": true,
	"file": true, "pwd": true, "which": true, "whereis": true, "type": true,
	"basename": true, "dirname": true, "realpath": true, "readlink": true,
	"sort": true, "uniq": true, "cut": true, "tr": true, "jq": true, "yq": true,
	"echo": true, "printf": true, "date": true, "uptime": true, "hostname": true,
	"test": true, "[": true, "diff": true, "comm": true, "column": true,
	"git":  true, // 子命令再细分
	"curl": true, // 仅限本机数据查询接口
	"wget": true, // 同上
}

// git 只读子命令。
var gitReadOnly = map[string]bool{
	"status": true, "log": true, "diff": true, "show": true, "blame": true,
	"branch": true, "ls-files": true, "ls-tree": true, "rev-parse": true,
	"describe": true, "remote": true, "config": false, "shortlog": true,
}

// catastrophic 高危子串(含管道到 shell)。
var catastrophic = []string{
	"rm -rf /", "rm -fr /", "no-preserve-root", "mkfs", "dd if=/dev/zero",
	"of=/dev/sd", ":(){", "shutdown", "reboot", " halt", "chown -r /",
	"chmod -r 777 /", "> /etc/", "> /boot/", "mv / ", "| sh", "| bash", "|zsh",
}

// pathKeys 各工具中表示目标路径的参数名。
var pathKeys = []string{"file_path", "path", "notebook_path", "dir_path", "notebook_path"}

// Classify 判定一次工具调用:只读放行、写/越权弹窗、灾难拒绝。
// roots 为已授权的 code 工作区绝对路径;为空时保守要求人工确认。
func Classify(tool string, input map[string]interface{}, roots []string) Decision {
	if isCatastrophic(tool, input) {
		return Decision{Deny, "命中高危命令黑名单,已拒绝"}
	}

	switch {
	case tool == "Bash":
		return classifyBash(str(input, "command"), roots)
	case tool == "WebFetch" || tool == "WebSearch":
		return Decision{Ask, "分身将访问外网(" + tool + "),需你确认"}
	case readOnlyTools[tool]:
		return classifyReadPath(tool, input, roots)
	case writeTools[tool]:
		return Decision{Ask, "写操作需你确认:" + tool + " " + briefPath(input)}
	default:
		return Decision{Ask, "未知工具 " + tool + ",需你确认"}
	}
}

// classifyReadPath 只读工具的路径授权:授权目录内放行,越权弹窗。
func classifyReadPath(tool string, input map[string]interface{}, roots []string) Decision {
	p := pathFromInput(input)
	if p == "" || !filepath.IsAbs(p) {
		// 相对路径落在工作目录(已设为授权工作区)内
		return Decision{Allow, ""}
	}
	for _, r := range roots {
		if within(p, r) {
			return Decision{Allow, ""}
		}
	}
	if len(roots) == 0 {
		return Decision{Ask, "未配置授权目录,读取 " + p + " 需你确认"}
	}
	return Decision{Ask, "越出授权目录:" + p}
}

// classifyBash 判定 shell 命令:纯只读链放行,含写/未知/联网弹窗。
func classifyBash(cmd string, roots []string) Decision {
	c := strings.TrimSpace(cmd)
	if c == "" {
		return Decision{Ask, "空命令,需你确认"}
	}
	low := strings.ToLower(c)

	// 重定向=写文件(允许丢弃 stderr)
	scrubbed := strings.ReplaceAll(low, "2>/dev/null", "")
	scrubbed = strings.ReplaceAll(scrubbed, "2>&1", "")
	if strings.Contains(scrubbed, ">") {
		return Decision{Ask, "命令含输出重定向(会写文件)"}
	}

	segs := splitSegments(c)
	if len(segs) == 0 {
		return Decision{Ask, "无法解析命令"}
	}
	for _, seg := range segs {
		fields := strings.Fields(seg)
		if len(fields) == 0 {
			continue
		}
		bin := filepath.Base(strings.Trim(fields[0], `"'`))
		if strings.HasPrefix(bin, "./") || strings.HasPrefix(bin, "/") {
			return Decision{Ask, "命令含本地可执行文件:" + fields[0]}
		}
		switch {
		case bin == "git":
			sub, extra := gitSubcommand(fields[1:], roots)
			if extra != "" {
				return Decision{Ask, extra}
			}
			if !gitReadOnly[sub] {
				return Decision{Ask, "git 写操作:git " + sub}
			}
		case bin == "sed":
			if hasInPlaceFlag(fields[1:]) {
				return Decision{Ask, "sed 原地修改文件(-i)"}
			}
		case bin == "find":
			if containsAny(lower(fields), "-delete", "-exec", "-execok", "-ok", "-fprint", "-fls") {
				return Decision{Ask, "find 带删除/执行动作"}
			}
		case bin == "curl" || bin == "wget":
			if !isLocalDataAPI(seg) {
				return Decision{Ask, "非本机数据查询的网络访问"}
			}
		case !readOnlyCmds[bin]:
			return Decision{Ask, "非只读命令:" + bin}
		}
	}
	return Decision{Allow, ""}
}

// isLocalDataAPI 仅允许访问本机后端只读数据查询接口。
func isLocalDataAPI(seg string) bool {
	low := strings.ToLower(seg)
	return strings.Contains(low, "127.0.0.1:8080/api/db/") || strings.Contains(low, "localhost:8080/api/db/")
}

// isCatastrophic 扫描明显破坏性命令。
func isCatastrophic(tool string, input map[string]interface{}) bool {
	var hay string
	if tool == "Bash" {
		hay = strings.ToLower(str(input, "command"))
	} else {
		hay = strings.ToLower(briefPath(input))
	}
	if hay == "" {
		return false
	}
	norm := strings.Join(strings.Fields(hay), " ")
	for _, bad := range catastrophic {
		if strings.Contains(norm, bad) {
			return true
		}
	}
	return false
}

// splitSegments 按 shell 控制元字符(; && || | &)与换行拆分为若干单命令。
func splitSegments(cmd string) []string {
	out := []string{}
	cur := strings.Builder{}
	flush := func() {
		s := strings.TrimSpace(cur.String())
		if s != "" {
			out = append(out, s)
		}
		cur.Reset()
	}
	rs := []rune(cmd)
	for i := 0; i < len(rs); i++ {
		ch := rs[i]
		if ch == '\n' || ch == ';' || ch == '|' || ch == '&' {
			// && || 连吃一个
			if i+1 < len(rs) && (rs[i+1] == ch) {
				i++
			}
			flush()
			continue
		}
		cur.WriteRune(ch)
	}
	flush()
	return out
}

// gitFlagsWithValue 是需要跟一个参数的 git 全局选项。
var gitFlagsWithValue = map[string]bool{"-C": true, "-c": true, "--git-dir": true, "--namespace": true}

// gitSubcommand 解析 git 全局选项后的子命令。
// 返回 (子命令, 需要人工确认的原因);原因非空表示不放行。
func gitSubcommand(args []string, roots []string) (string, string) {
	i := 0
	for i < len(args) {
		a := args[i]
		if !strings.HasPrefix(a, "-") {
			return a, ""
		}
		// -C<path> / --git-dir=<path> 内联形式
		key := a
		val := ""
		if j := strings.Index(a, "="); j >= 0 {
			key, val = a[:j], a[j+1:]
		} else if gitFlagsWithValue[a] && i+1 < len(args) {
			val = args[i+1]
			i++
		}
		if key == "-c" || key == "--config-env" || key == "-p" {
			// -c 可注入 core.sshCommand 等危险配置,一律确认
			return "", "git 使用 -c 注入配置,需确认"
		}
		if key == "--git-dir" || key == "-C" {
			if v := checkGitDir(val, roots); v != "" {
				return "", v
			}
		}
		i++
	}
	return "", "无法确定 git 子命令"
}

// checkGitDir 校验 git -C 目标目录在授权范围内。
func checkGitDir(p string, roots []string) string {
	if p == "" {
		return ""
	}
	abs := p
	if !filepath.IsAbs(abs) {
		return "git -C 使用相对路径,需确认" // 相对 cwd 无法静态判定
	}
	for _, r := range roots {
		if within(abs, r) {
			return ""
		}
	}
	return "git -C 越出授权目录:" + p
}

// hasInPlaceFlag 判断 sed 是否带 -i / --in-place。
func hasInPlaceFlag(args []string) bool {
	for _, a := range args {
		if a == "--in-place" || strings.HasPrefix(a, "--in-place=") {
			return true
		}
		if strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && strings.Contains(a, "i") {
			return true
		}
	}
	return false
}

func containsAny(hay []string, needles ...string) bool {
	for _, h := range hay {
		l := strings.ToLower(h)
		for _, n := range needles {
			if strings.Contains(l, n) {
				return true
			}
		}
	}
	return false
}

func lower(s []string) []string {
	out := make([]string, len(s))
	for i, v := range s {
		out[i] = strings.ToLower(v)
	}
	return out
}

// pathFromInput 取输入中的路径参数。
func pathFromInput(input map[string]interface{}) string {
	for _, k := range pathKeys {
		if v := str(input, k); v != "" {
			return v
		}
	}
	return ""
}

// within 判定 abs 是否位于 root 之内(含 root 本身)。
func within(abs, root string) bool {
	a := filepath.Clean(abs)
	r := filepath.Clean(root)
	if a == r {
		return true
	}
	rel, err := filepath.Rel(r, a)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// briefPath 生成用于展示的目标路径摘要(无路径时返回空串)。
func briefPath(input map[string]interface{}) string {
	if p := pathFromInput(input); p != "" {
		return p
	}
	return ""
}

// str 安全取字符串字段。
func str(input map[string]interface{}, key string) string {
	if input == nil {
		return ""
	}
	if v, ok := input[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
