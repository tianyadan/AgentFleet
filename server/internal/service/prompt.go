package service

import (
	"context"

	"colleague-avatar/server/internal/agent"
)

// SystemPrompt 组装分身系统提示词(人设 + 只读 + 注入防御 + 数据查询)。
func (s *Service) SystemPrompt(ctx context.Context) string {
	persona := agent.Persona{Name: "E-bot 数字员工", Style: "严谨、简洁、专业"}
	ws, _ := s.Store.CodeWorkspaces(ctx)
	roots := make([]string, 0, len(ws))
	for _, w := range ws {
		roots = append(roots, w.Path)
	}
	p := agent.SystemPrompt(persona, roots)
	// 数据查询项目只在显式配置 DSN 后可用,避免默认连接真实环境。
	if s.DBQuery != nil {
		p += "\n\n【数据查询项目】\n"
		p += "- 用户可能问业务数据(例如 data-link 或 kfi-cloud 库中的记录)。这些是授权的数据项目,属于你的可查询范围。\n"
		p += "- 通过调用本机后端接口查询,只读:\n"
		p += "  - `curl -s http://127.0.0.1:8080/api/db/databases` 列库\n"
		p += "  - `curl -s 'http://127.0.0.1:8080/api/db/tables?database=kfi-cloud'` 列表\n"
		p += "  - `curl -s -XPOST http://127.0.0.1:8080/api/db/query -H 'Content-Type: application/json' -d '{\"database\":\"kfi-cloud\",\"sql\":\"SELECT ...\"}'` 查询(仅只读 SQL)\n"
		p += "- 查询结果要整理成易读的 markdown 表格/列表返回给用户。仅回答有权限的数据,严禁查询 data-link、kfi-cloud 以外的库。\n"
		p += "- 数据查询同样只读:不得 INSERT/UPDATE/DELETE/DROP 等。\n"
	}
	p += "\n\n【多 Agent 任务进度】\n"
	p += "- 其他 AI(cursor/codex/claude 等)可通过 POST /api/agents/tasks 上报任务进度。\n"
	p += "- 用户问「某某任务做到哪了/多 agent 进度」时,用只读 curl 查询后整理成列表回答:\n"
	p += "  - `curl -s http://127.0.0.1:8080/api/agents/tasks`\n"
	p += "  - 可加过滤: `?agent_type=cursor&status=running`\n"
	p += "- 字段含 agent_type/task_id/task_name/status/progress/message/updated_at。\n"
	return p
}
