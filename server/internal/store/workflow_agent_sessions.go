package store

import (
	"context"
	"database/sql"
	"time"
)

// WorkflowAgentSession 项目协作中「项目 × 数字员工」持久会话绑定。
type WorkflowAgentSession struct {
	ID             int64     `json:"id"`
	DefinitionID   int64     `json:"definition_id"`
	AgentID        int64     `json:"agent_id"`
	ConversationID int64     `json:"conversation_id"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// GetWorkflowAgentConversation 读取绑定的协作会话；无则 0。
func (s *Store) GetWorkflowAgentConversation(ctx context.Context, defID, agentID int64) (int64, error) {
	if defID <= 0 || agentID <= 0 {
		return 0, nil
	}
	var convID int64
	err := s.db.QueryRowContext(ctx,
		`SELECT conversation_id FROM workflow_agent_sessions WHERE definition_id=? AND agent_id=?`,
		defID, agentID).Scan(&convID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return convID, err
}

// UpsertWorkflowAgentSession 写入或更新项目×员工会话绑定。
func (s *Store) UpsertWorkflowAgentSession(ctx context.Context, defID, agentID, convID int64) error {
	if defID <= 0 || agentID <= 0 || convID <= 0 {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO workflow_agent_sessions (definition_id, agent_id, conversation_id)
		VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE conversation_id=VALUES(conversation_id), updated_at=CURRENT_TIMESTAMP`,
		defID, agentID, convID)
	return err
}

// ListWorkflowAgentSessions 列出某项目下全部员工协作会话。
func (s *Store) ListWorkflowAgentSessions(ctx context.Context, defID int64) ([]WorkflowAgentSession, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, definition_id, agent_id, conversation_id, created_at, updated_at
		FROM workflow_agent_sessions WHERE definition_id=? ORDER BY id`, defID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WorkflowAgentSession
	for rows.Next() {
		var it WorkflowAgentSession
		if err := rows.Scan(&it.ID, &it.DefinitionID, &it.AgentID, &it.ConversationID, &it.CreatedAt, &it.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	if out == nil {
		out = []WorkflowAgentSession{}
	}
	return out, rows.Err()
}

// DeleteWorkflowAgentSessions 删除某项目全部绑定（归档完成后或定义删除前）。
func (s *Store) DeleteWorkflowAgentSessions(ctx context.Context, defID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM workflow_agent_sessions WHERE definition_id=?`, defID)
	return err
}
