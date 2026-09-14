package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Activity 一次可展示的运行态(工具调用/状态)，不含 diff 杂音。
type Activity struct {
	Tool    string `json:"tool"`
	Summary string `json:"summary"`
}

// ActivityFn 流式活动回调。
type ActivityFn func(Activity)

// SummarizeToolActivity 把工具名+入参压成一行状态(不展开文件内容/diff)。
func SummarizeToolActivity(tool string, input map[string]interface{}) Activity {
	tool = strings.TrimSpace(tool)
	if tool == "" {
		tool = "Tool"
	}
	switch tool {
	case "Bash", "bash", "Shell":
		cmd := strField(input, "command")
		if cmd == "" {
			return Activity{Tool: tool, Summary: "正在执行命令…"}
		}
		return Activity{Tool: tool, Summary: "执行命令 · " + truncateRunes(cmd, 72)}
	case "Read", "NotebookRead":
		return Activity{Tool: tool, Summary: "正在读取…"}
	case "Write", "Edit", "MultiEdit", "NotebookEdit":
		return Activity{Tool: tool, Summary: "正在编辑…"}
	case "Grep", "Glob", "LS":
		return Activity{Tool: tool, Summary: "正在搜索…"}
	case "WebFetch", "WebSearch":
		return Activity{Tool: tool, Summary: "正在联网…"}
	case "Browser", "browser-use":
		return Activity{Tool: tool, Summary: "正在操作浏览器…"}
	default:
		return Activity{Tool: tool, Summary: "调用 " + tool + "…"}
	}
}

// ParseStreamActivity 从 Claude stream-json 一行尝试提取工具活动。
func ParseStreamActivity(line []byte) (Activity, bool) {
	var raw map[string]interface{}
	if json.Unmarshal(line, &raw) != nil {
		return Activity{}, false
	}
	// stream_event → content_block_start → tool_use
	if ev, ok := raw["event"].(map[string]interface{}); ok {
		if strField(ev, "type") == "content_block_start" {
			if cb, ok := ev["content_block"].(map[string]interface{}); ok {
				if strField(cb, "type") == "tool_use" {
					name := strField(cb, "name")
					in, _ := cb["input"].(map[string]interface{})
					return SummarizeToolActivity(name, in), true
				}
			}
		}
	}
	// assistant message content tool_use
	if strField(raw, "type") == "assistant" {
		msg, _ := raw["message"].(map[string]interface{})
		if msg == nil {
			return Activity{}, false
		}
		arr, _ := msg["content"].([]interface{})
		for _, it := range arr {
			m, _ := it.(map[string]interface{})
			if m == nil {
				continue
			}
			if strField(m, "type") == "tool_use" {
				name := strField(m, "name")
				in, _ := m["input"].(map[string]interface{})
				return SummarizeToolActivity(name, in), true
			}
		}
	}
	return Activity{}, false
}

func strField(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

func truncateRunes(s string, n int) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + "…"
}
