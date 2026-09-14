package store

import (
	"context"
	"database/sql"
	"time"
)

// CommandAudit 一条命令审批/执行审计记录。
type CommandAudit struct {
	ID             int64     `json:"id"`
	ConversationID int64     `json:"conversation_id"`
	AgentID        int64     `json:"agent_id"`
	ToolName       string    `json:"tool_name"`
	CommandText    string    `json:"command_text"`
	Decision       string    `json:"decision"`   // allow | deny
	DecidedBy      string    `json:"decided_by"` // user | ai | system | timeout | disconnect
	Risk           string    `json:"risk"`
	Meaning        string    `json:"meaning"`
	Note           string    `json:"note"`
	CreatedAt      time.Time `json:"created_at"`
}

// InsertCommandAudit 写入命令审计。
func (s *Store) InsertCommandAudit(ctx context.Context, a *CommandAudit) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO command_audits
		 (conversation_id, agent_id, tool_name, command_text, decision, decided_by, risk, meaning, note)
		 VALUES (?,?,?,?,?,?,?,?,?)`,
		a.ConversationID, a.AgentID, a.ToolName, a.CommandText, a.Decision, a.DecidedBy, a.Risk, a.Meaning, a.Note)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListCommandAudits 按会话或智能体查审计(优先 conversation_id)。
func (s *Store) ListCommandAudits(ctx context.Context, conversationID, agentID int64, limit int) ([]CommandAudit, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := `SELECT id, conversation_id, agent_id, tool_name, command_text, decision, decided_by,
	             IFNULL(risk,''), IFNULL(meaning,''), IFNULL(note,''), created_at
	      FROM command_audits WHERE 1=1`
	args := []interface{}{}
	if conversationID > 0 {
		q += ` AND conversation_id=?`
		args = append(args, conversationID)
	} else if agentID > 0 {
		q += ` AND agent_id=?`
		args = append(args, agentID)
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CommandAudit
	for rows.Next() {
		var a CommandAudit
		if err := rows.Scan(&a.ID, &a.ConversationID, &a.AgentID, &a.ToolName, &a.CommandText,
			&a.Decision, &a.DecidedBy, &a.Risk, &a.Meaning, &a.Note, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ConversationAgentID 返回会话绑定的 managed agent id(无则 0)。
func (s *Store) ConversationAgentID(ctx context.Context, conversationID int64) (int64, error) {
	var id sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT agent_id FROM conversations WHERE id=?`, conversationID).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if !id.Valid {
		return 0, nil
	}
	return id.Int64, nil
}
