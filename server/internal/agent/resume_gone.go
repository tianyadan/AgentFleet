package agent

import "strings"

// IsCodexResumeGone 判断是否为「thread 已失效 / 本机无 rollout」类错误。
// 常见于：本地 sessions 被清理、thread 过期，或 DB 仍粘着旧 engine_session_id。
func IsCodexResumeGone(msg string) bool {
	m := strings.ToLower(strings.TrimSpace(msg))
	if m == "" {
		return false
	}
	if strings.Contains(m, "no rollout found") {
		return true
	}
	if strings.Contains(m, "thread/resume") && strings.Contains(m, "fail") {
		return true
	}
	if strings.Contains(m, "rollout") && strings.Contains(m, "not found") {
		return true
	}
	return false
}
