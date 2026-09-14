package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

// Occupancy 智能体当前主任务占用。
type Occupancy struct {
	AgentID         int64     `json:"agent_id"`
	SourceType      string    `json:"source_type"`
	SourceID        string    `json:"source_id"`
	SourceName      string    `json:"source_name"`
	WorkflowRunID   int64     `json:"workflow_run_id"`
	NodeExecutionID int64     `json:"node_execution_id"`
	TaskName        string    `json:"task_name"`
	StartedAt       time.Time `json:"started_at"`
}

// TryInsertOccupancy 占用成功返回 true；已占用返回 false。
func (s *Store) TryInsertOccupancy(ctx context.Context, o Occupancy) (bool, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT IGNORE INTO agent_occupancy
		(agent_id, source_type, source_id, source_name, workflow_run_id, node_execution_id, task_name, started_at)
		VALUES (?,?,?,?,?,?,?,NOW())`,
		o.AgentID, o.SourceType, o.SourceID, o.SourceName, o.WorkflowRunID, o.NodeExecutionID, o.TaskName)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// GetOccupancy 查询占用；无则 nil。
func (s *Store) GetOccupancy(ctx context.Context, agentID int64) (*Occupancy, error) {
	var o Occupancy
	err := s.db.QueryRowContext(ctx, `
		SELECT agent_id, source_type, source_id, source_name, workflow_run_id, node_execution_id, task_name, started_at
		FROM agent_occupancy WHERE agent_id=?`, agentID).Scan(
		&o.AgentID, &o.SourceType, &o.SourceID, &o.SourceName, &o.WorkflowRunID, &o.NodeExecutionID, &o.TaskName, &o.StartedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// ListOccupancyMap 批量占用 map[agentID]。
func (s *Store) ListOccupancyMap(ctx context.Context) (map[int64]Occupancy, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT agent_id, source_type, source_id, source_name, workflow_run_id, node_execution_id, task_name, started_at
		FROM agent_occupancy`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]Occupancy{}
	for rows.Next() {
		var o Occupancy
		if err := rows.Scan(&o.AgentID, &o.SourceType, &o.SourceID, &o.SourceName, &o.WorkflowRunID, &o.NodeExecutionID, &o.TaskName, &o.StartedAt); err != nil {
			return nil, err
		}
		out[o.AgentID] = o
	}
	return out, rows.Err()
}

// ClearOccupancy 释放占用。
func (s *Store) ClearOccupancy(ctx context.Context, agentID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM agent_occupancy WHERE agent_id=?`, agentID)
	return err
}

// ClearAllOccupancy 清空全部占用(服务重启)。
func (s *Store) ClearAllOccupancy(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM agent_occupancy`)
	return err
}

// AgentMemory 长期记忆条目。
type AgentMemory struct {
	ID           int64     `json:"id"`
	AgentID      int64     `json:"agent_id"`
	SourceType   string    `json:"source_type"`
	SourceID     string    `json:"source_id"`
	WorkflowID   int64     `json:"workflow_id"`
	Workspace    string    `json:"workspace"`
	Title        string    `json:"title"`
	Summary      string    `json:"summary"`
	MetadataJSON string    `json:"metadata_json,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// InsertAgentMemory 写入记忆。
func (s *Store) InsertAgentMemory(ctx context.Context, m *AgentMemory) (int64, error) {
	meta := m.MetadataJSON
	if strings.TrimSpace(meta) == "" {
		meta = "{}"
	}
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO agent_memories
		(agent_id, source_type, source_id, workflow_id, workspace, title, summary, metadata_json)
		VALUES (?,?,?,?,?,?,?,CAST(? AS JSON))`,
		m.AgentID, m.SourceType, m.SourceID, m.WorkflowID, m.Workspace, m.Title, m.Summary, meta)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListAgentMemoriesRecent 最近记忆（召回候选池）。
func (s *Store) ListAgentMemoriesRecent(ctx context.Context, agentID int64, limit int) ([]AgentMemory, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, agent_id, source_type, source_id, workflow_id, workspace, title, summary,
		       CAST(metadata_json AS CHAR), created_at
		FROM agent_memories WHERE agent_id=? ORDER BY created_at DESC LIMIT ?`, agentID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AgentMemory
	for rows.Next() {
		var m AgentMemory
		var meta sql.NullString
		if err := rows.Scan(&m.ID, &m.AgentID, &m.SourceType, &m.SourceID, &m.WorkflowID, &m.Workspace,
			&m.Title, &m.Summary, &meta, &m.CreatedAt); err != nil {
			return nil, err
		}
		if meta.Valid {
			m.MetadataJSON = meta.String
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// MustJSON 方便写入；非法则 "{}"。
func MustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}
