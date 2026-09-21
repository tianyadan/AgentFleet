package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"colleague-avatar/server/internal/store"
)

// SafeProjectDirName 将项目名转为可作目录名的安全片段。
func SafeProjectDirName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "unnamed-project"
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r >= 0x4e00:
			b.WriteRune(r)
		case unicode.IsSpace(r) || r == '/' || r == '\\' || r == ':' || r == '.':
			b.WriteByte('_')
		default:
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "unnamed-project"
	}
	if len(out) > 80 {
		out = out[:80]
	}
	return out
}

// TruncateRunes 按 rune 截断。
func TruncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	n := 0
	for i := range s {
		if n == max {
			return s[:i]
		}
		n++
	}
	return s
}

// GetOrCreateWorkflowAgentConversation 项目×员工持久协作会话（不改主会话指针）。
func (s *Service) GetOrCreateWorkflowAgentConversation(ctx context.Context, defID, agentID int64) (int64, error) {
	if defID <= 0 || agentID <= 0 {
		return 0, fmt.Errorf("invalid definition or agent")
	}
	convID, err := s.Store.GetWorkflowAgentConversation(ctx, defID, agentID)
	if err != nil {
		return 0, err
	}
	if convID > 0 {
		return convID, nil
	}
	convID, err = s.Store.CreateAgentConversation(ctx, agentID)
	if err != nil {
		return 0, err
	}
	if err := s.Store.UpsertWorkflowAgentSession(ctx, defID, agentID, convID); err != nil {
		return 0, err
	}
	return convID, nil
}

// ArchiveWorkflowProjectMemories 解散前：为每位员工写 projects/<名>/summary.md。
func (s *Service) ArchiveWorkflowProjectMemories(ctx context.Context, def *store.WorkflowDefinition) error {
	if def == nil {
		return nil
	}
	sessions, err := s.Store.ListWorkflowAgentSessions(ctx, def.ID)
	if err != nil {
		return err
	}
	var firstErr error
	for _, sess := range sessions {
		a, err := s.Store.GetManagedAgent(ctx, sess.AgentID)
		if err != nil || a == nil {
			continue
		}
		summary, sumErr := s.summarizeProjectSession(ctx, a, sess.ConversationID, def.Name)
		if sumErr != nil || strings.TrimSpace(summary) == "" {
			summary = "（未能自动生成总结；会话元数据已写入归档文件。）"
			if sumErr != nil {
				summary = "（总结失败: " + sumErr.Error() + "）"
				if firstErr == nil {
					firstErr = sumErr
				}
			}
		}
		summary = TruncateRunes(strings.TrimSpace(summary), 300)
		meta, _ := s.Store.GetConversationEngineMeta(ctx, sess.ConversationID)
		if err := s.writeProjectSummaryFile(a, def.Name, sess, meta.SessionID, summary); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (s *Service) summarizeProjectSession(ctx context.Context, a *store.ManagedAgent, convID int64, projectName string) (string, error) {
	sys := "你是工作总结助手。只输出简洁中文摘要正文，不要超过300字，不要标题堆砌。"
	q := fmt.Sprintf("项目「%s」即将解散。请根据当前协作会话中你做过的事，写一段不超过300字的总结：做了什么、关键结论、未完成事项。只输出摘要正文。", projectName)
	// 使用协作会话 resume；走正常 hook/resume 路径（不改主会话指针）
	out, _, err := s.runManagedEngine(ctx, a, sys, q, "", nil, nil, convID, a.ID, false, nil)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (s *Service) writeProjectSummaryFile(a *store.ManagedAgent, projectName string, sess store.WorkflowAgentSession, engineSessionID, summary string) error {
	root := strings.TrimSpace(a.WorkspacePath)
	if root == "" {
		root = s.Cfg.WorkspaceRoot
	}
	dir := filepath.Join(root, "projects", SafeProjectDirName(projectName))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	body := fmt.Sprintf(`# 项目协作归档：%s

- 数字员工：%s (id=%d)
- 归档时间：%s
- 平台 conversation_id：%d
- 引擎 session_id：%s
- 工作流 definition_id：%d

## 工作总结

%s
`,
		projectName,
		a.Name, a.ID,
		time.Now().Format("2006-01-02 15:04:05"),
		sess.ConversationID,
		strings.TrimSpace(engineSessionID),
		sess.DefinitionID,
		summary,
	)
	return os.WriteFile(filepath.Join(dir, "summary.md"), []byte(body), 0o644)
}

// ProjectMemoryHint 注入系统提示：可查阅工作区项目归档。
func ProjectMemoryHint() string {
	return "【项目协作记忆】若用户询问历史项目、曾经做过什么，可查阅工作区目录 projects/*/summary.md（及同目录其它文件）后简要回答；无需罗列全部文件，抓住要点即可。"
}
