package store

import (
	"context"
	"database/sql"
	"strings"
)

// AgentFolder 数字员工工作组（DB 表仍为 agent_folders）。
type AgentFolder struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	SortOrder int    `json:"sort_order"`
	CreatedAt string `json:"created_at,omitempty"`
}

// ListAgentFolders 工作组列表（排序号升序）。
func (s *Store) ListAgentFolders(ctx context.Context) ([]AgentFolder, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, sort_order, created_at FROM agent_folders ORDER BY sort_order ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AgentFolder
	for rows.Next() {
		var f AgentFolder
		var created sql.NullTime
		if err := rows.Scan(&f.ID, &f.Name, &f.SortOrder, &created); err != nil {
			return nil, err
		}
		if created.Valid {
			f.CreatedAt = created.Time.Format("2006-01-02T15:04:05Z07:00")
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// CreateAgentFolder 创建空工作组。
func (s *Store) CreateAgentFolder(ctx context.Context, name string) (int64, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "新建工作组"
	}
	var maxSort sql.NullInt64
	_ = s.db.QueryRowContext(ctx, `SELECT MAX(sort_order) FROM agent_folders`).Scan(&maxSort)
	next := int(maxSort.Int64) + 1
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO agent_folders (name, sort_order) VALUES (?, ?)`, name, next)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// RenameAgentFolder 重命名工作组。
func (s *Store) RenameAgentFolder(ctx context.Context, id int64, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return sql.ErrNoRows
	}
	res, err := s.db.ExecContext(ctx, `UPDATE agent_folders SET name=? WHERE id=?`, name, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteAgentFolder 删除工作组并把其下数字员工移到未入组。
func (s *Store) DeleteAgentFolder(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE managed_agents SET folder_id=NULL WHERE folder_id=?`, id); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM agent_folders WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return tx.Commit()
}

// SetManagedAgentFolder 将数字员工放入工作组；folderID=0 表示未入组。
func (s *Store) SetManagedAgentFolder(ctx context.Context, agentID, folderID int64) error {
	var arg any
	if folderID > 0 {
		arg = folderID
	} else {
		arg = nil
	}
	res, err := s.db.ExecContext(ctx, `UPDATE managed_agents SET folder_id=? WHERE id=?`, arg, agentID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ConversationStats 会话占用粗估（状态面板用，不入库）。
type ConversationStats struct {
	MessageCount int   `json:"message_count"`
	UserCount    int   `json:"user_count"`
	AssistantCount int `json:"assistant_count"`
	CharCount    int64 `json:"char_count"`
}

// GetConversationStats 统计会话消息数与字符量。
func (s *Store) GetConversationStats(ctx context.Context, conversationID int64) (ConversationStats, error) {
	var st ConversationStats
	if conversationID <= 0 {
		return st, nil
	}
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*),
		       IFNULL(SUM(CASE WHEN role='user' THEN 1 ELSE 0 END),0),
		       IFNULL(SUM(CASE WHEN role='assistant' THEN 1 ELSE 0 END),0),
		       IFNULL(SUM(CHAR_LENGTH(IFNULL(content,''))),0)
		FROM messages WHERE conversation_id=? AND IFNULL(exclude_from_context,0)=0`, conversationID).
		Scan(&st.MessageCount, &st.UserCount, &st.AssistantCount, &st.CharCount)
	return st, err
}
