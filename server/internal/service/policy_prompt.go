package service

import (
	"fmt"
	"strings"

	"colleague-avatar/server/internal/store"
)

// BuildAgentSystemPrompt 拼系统提示 + 权限策略硬约束。
func BuildAgentSystemPrompt(a *store.ManagedAgent) string {
	base := strings.TrimSpace(a.RulesPrompt)
	if base == "" {
		base = "你是管理台智能体「" + a.Name + "」,按用户指令完成开发/排查任务。回答简洁专业。"
	}
	return base + "\n\n" + AgentPolicyBlock(a)
}

// AgentPolicyBlock 权限策略条文(始终注入)。
func AgentPolicyBlock(a *store.ManagedAgent) string {
	var b strings.Builder
	b.WriteString("【智能体权限策略 — 必须遵守】\n")
	if a.AllowWrite {
		b.WriteString("- 写入权限:允许修改/创建文件(仍可能需用户授权弹窗)。\n")
	} else {
		b.WriteString("- 写入权限:禁止。不得创建/修改/删除文件,不得重定向写入。\n")
	}
	if a.AllowNetwork {
		b.WriteString("- 联网权限:允许访问外网/搜索(仍可能需授权)。\n")
	} else {
		b.WriteString("- 联网权限:禁止。不得 curl/wget/WebFetch/WebSearch 等外网访问。\n")
	}
	if a.AllowRm {
		b.WriteString("- rm 权限:允许执行 rm(仍可能需授权)。\n")
	} else {
		b.WriteString("- rm 权限:禁止。不得执行任何 rm/unlink 删除命令。\n")
	}
	if a.AllowBrowser {
		b.WriteString("- 浏览器权限:允许。可使用 browser-use 相关工具操作浏览器完成网页交互、点击、填表、抓取页面等。\n")
		b.WriteString("  前提:本机已安装 browser-use 插件/能力；操作时说明步骤，注意勿泄露凭据。\n")
		b.WriteString("  重要:使用完成网页之后要及时关闭网页，不要留有僵尸网页。\n")
	} else {
		b.WriteString("- 浏览器权限:禁止。不得调用 browser-use / 浏览器自动化工具。\n")
	}
	ws := strings.TrimSpace(a.WorkspacePath)
	if ws == "" {
		b.WriteString("- 工作区:未绑定,可在已授权工作区范围内跨项目工作。\n")
	} else {
		b.WriteString("- 工作区:已绑定「" + ws + "」。严禁读写该目录之外的任何路径。\n")
	}
	b.WriteString("违规操作会被系统直接拒绝。")
	return b.String()
}

// MaybeReinforcePolicy 每 10 轮用户消息附加策略重申。
func MaybeReinforcePolicy(a *store.ManagedAgent, userTurnCount int, question string) string {
	if userTurnCount > 0 && userTurnCount%10 == 0 {
		return "【策略重申 — 第" + fmt.Sprintf("%d", userTurnCount) + "轮】\n" + AgentPolicyBlock(a) + "\n\n" + question
	}
	return question
}
