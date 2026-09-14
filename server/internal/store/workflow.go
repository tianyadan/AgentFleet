package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

// WorkflowDefinition 可复用编排模板。
type WorkflowDefinition struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	GraphJSON   string    `json:"graph_json"`
	Version     int       `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// WorkflowRun 一次运行。
type WorkflowRun struct {
	ID                 int64      `json:"id"`
	DefinitionID       int64      `json:"definition_id"`
	Status             string     `json:"status"`
	InputPrompt        string     `json:"input_prompt"`
	CurrentNodeIDsJSON string     `json:"current_node_ids_json,omitempty"`
	ParallelStateJSON  string     `json:"parallel_state_json,omitempty"`
	FailNodeID         string     `json:"fail_node_id,omitempty"`
	FailReason         string     `json:"fail_reason,omitempty"`
	ProgressJSON       string     `json:"progress_json,omitempty"`
	ResumeCursorJSON   string     `json:"resume_cursor_json,omitempty"`
	StartedAt          *time.Time `json:"started_at,omitempty"`
	FinishedAt         *time.Time `json:"finished_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
}

// NodeExecution 节点一次 attempt。
type NodeExecution struct {
	ID              int64      `json:"id"`
	RunID           int64      `json:"run_id"`
	NodeID          string     `json:"node_id"`
	Attempt         int        `json:"attempt"`
	NodeType        string     `json:"node_type"`
	AgentID         int64      `json:"agent_id"`
	Status          string     `json:"status"`
	ConversationID  int64      `json:"conversation_id"`
	InputJSON       string     `json:"input_json,omitempty"`
	OutputJSON      string     `json:"output_json,omitempty"`
	EventsJSON      string     `json:"events_json,omitempty"`
	ErrorText       string     `json:"error_text,omitempty"`
	ParentAttemptID int64      `json:"parent_attempt_id,omitempty"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

// CreateWorkflowDefinition 新建编排。
func (s *Store) CreateWorkflowDefinition(ctx context.Context, name, desc, graphJSON string) (int64, error) {
	if strings.TrimSpace(graphJSON) == "" {
		graphJSON = `{"nodes":[],"edges":[]}`
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO workflow_definitions (name, description, graph_json) VALUES (?,?,CAST(? AS JSON))`,
		name, desc, graphJSON)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateWorkflowDefinition 更新编排。
func (s *Store) UpdateWorkflowDefinition(ctx context.Context, id int64, name, desc, graphJSON string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE workflow_definitions SET name=?, description=?, graph_json=CAST(? AS JSON), version=version+1
		WHERE id=?`, name, desc, graphJSON, id)
	return err
}

// DeleteWorkflowDefinition 删除编排(级联 runs)。
func (s *Store) DeleteWorkflowDefinition(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM workflow_definitions WHERE id=?`, id)
	return err
}

// GetWorkflowDefinition 按 ID。
func (s *Store) GetWorkflowDefinition(ctx context.Context, id int64) (*WorkflowDefinition, error) {
	var d WorkflowDefinition
	var graph []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, COALESCE(description,''), graph_json, version, created_at, updated_at
		FROM workflow_definitions WHERE id=?`, id).Scan(
		&d.ID, &d.Name, &d.Description, &graph, &d.Version, &d.CreatedAt, &d.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	d.GraphJSON = string(graph)
	return &d, nil
}

// ListWorkflowDefinitions 列表(新在前)。
func (s *Store) ListWorkflowDefinitions(ctx context.Context) ([]WorkflowDefinition, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, COALESCE(description,''), graph_json, version, created_at, updated_at
		FROM workflow_definitions ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WorkflowDefinition
	for rows.Next() {
		var d WorkflowDefinition
		var graph []byte
		if err := rows.Scan(&d.ID, &d.Name, &d.Description, &graph, &d.Version, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		d.GraphJSON = string(graph)
		out = append(out, d)
	}
	if out == nil {
		out = []WorkflowDefinition{}
	}
	return out, rows.Err()
}

// ListLatestRunsByDefinitions 每个定义取最新一条 run（用于列表状态）。
func (s *Store) ListLatestRunsByDefinitions(ctx context.Context, defIDs []int64) (map[int64]*WorkflowRun, error) {
	out := map[int64]*WorkflowRun{}
	if len(defIDs) == 0 {
		return out, nil
	}
	// 逐个取最新（定义数量通常不大）
	for _, id := range defIDs {
		runs, err := s.ListWorkflowRuns(ctx, id, 1)
		if err != nil {
			return nil, err
		}
		if len(runs) > 0 {
			r := runs[0]
			// 补全 current_node 等字段
			full, err := s.GetWorkflowRun(ctx, r.ID)
			if err == nil && full != nil {
				out[id] = full
			} else {
				out[id] = &r
			}
		}
	}
	return out, nil
}

// CreateWorkflowRun 创建运行。
func (s *Store) CreateWorkflowRun(ctx context.Context, defID int64, prompt string) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO workflow_runs (definition_id, status, input_prompt, started_at)
		VALUES (?,'pending',?,NOW())`, defID, prompt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetWorkflowRun 查询运行。
func (s *Store) GetWorkflowRun(ctx context.Context, id int64) (*WorkflowRun, error) {
	var r WorkflowRun
	var cur, par, prog, resume sql.NullString
	var started, finished sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT id, definition_id, status, COALESCE(input_prompt,''),
		       current_node_ids_json, parallel_state_json, fail_node_id, COALESCE(fail_reason,''),
		       progress_json, resume_cursor_json, started_at, finished_at, created_at
		FROM workflow_runs WHERE id=?`, id).Scan(
		&r.ID, &r.DefinitionID, &r.Status, &r.InputPrompt,
		&cur, &par, &r.FailNodeID, &r.FailReason,
		&prog, &resume, &started, &finished, &r.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if cur.Valid {
		r.CurrentNodeIDsJSON = cur.String
	}
	if par.Valid {
		r.ParallelStateJSON = par.String
	}
	if prog.Valid {
		r.ProgressJSON = prog.String
	}
	if resume.Valid {
		r.ResumeCursorJSON = resume.String
	}
	if started.Valid {
		t := started.Time
		r.StartedAt = &t
	}
	if finished.Valid {
		t := finished.Time
		r.FinishedAt = &t
	}
	return &r, nil
}

// ListWorkflowRuns 某定义的最近运行。
func (s *Store) ListWorkflowRuns(ctx context.Context, defID int64, limit int) ([]WorkflowRun, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, definition_id, status, COALESCE(input_prompt,''), fail_node_id, COALESCE(fail_reason,''),
		       started_at, finished_at, created_at
		FROM workflow_runs WHERE definition_id=? ORDER BY id DESC LIMIT ?`, defID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WorkflowRun
	for rows.Next() {
		var r WorkflowRun
		var started, finished sql.NullTime
		if err := rows.Scan(&r.ID, &r.DefinitionID, &r.Status, &r.InputPrompt, &r.FailNodeID, &r.FailReason,
			&started, &finished, &r.CreatedAt); err != nil {
			return nil, err
		}
		if started.Valid {
			t := started.Time
			r.StartedAt = &t
		}
		if finished.Valid {
			t := finished.Time
			r.FinishedAt = &t
		}
		out = append(out, r)
	}
	if out == nil {
		out = []WorkflowRun{}
	}
	return out, rows.Err()
}

// UpdateWorkflowRunStatus 更新运行状态与失败信息。
func (s *Store) UpdateWorkflowRunStatus(ctx context.Context, id int64, status, failNode, failReason string, currentJSON, progressJSON, parallelJSON string) error {
	fin := status == "success" || status == "failed" || status == "stopped" || status == "interrupted"
	q := `UPDATE workflow_runs SET status=?, fail_node_id=?, fail_reason=?,
		current_node_ids_json=CAST(? AS JSON), progress_json=CAST(? AS JSON), parallel_state_json=CAST(? AS JSON)`
	args := []any{status, failNode, failReason, nullJSON(currentJSON), nullJSON(progressJSON), nullJSON(parallelJSON)}
	if fin {
		q += `, finished_at=NOW()`
	}
	q += ` WHERE id=?`
	args = append(args, id)
	_, err := s.db.ExecContext(ctx, q, args...)
	return err
}

func nullJSON(s string) string {
	if strings.TrimSpace(s) == "" {
		return "null"
	}
	return s
}

// InsertNodeExecution 新建 attempt。
func (s *Store) InsertNodeExecution(ctx context.Context, ne *NodeExecution) (int64, error) {
	in := ne.InputJSON
	if strings.TrimSpace(in) == "" {
		in = "{}"
	}
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO node_executions
		(run_id, node_id, attempt, node_type, agent_id, status, conversation_id, input_json, parent_attempt_id, started_at)
		VALUES (?,?,?,?,?,?,?,CAST(? AS JSON),?,NOW())`,
		ne.RunID, ne.NodeID, ne.Attempt, ne.NodeType, ne.AgentID, ne.Status, ne.ConversationID, in,
		nullInt64(ne.ParentAttemptID))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func nullInt64(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

// UpdateNodeExecution 更新节点执行。
func (s *Store) UpdateNodeExecution(ctx context.Context, id int64, status, outJSON, errText string, convID int64) error {
	out := outJSON
	if strings.TrimSpace(out) == "" {
		out = "null"
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE node_executions SET status=?, output_json=CAST(? AS JSON), error_text=?,
		conversation_id=IFNULL(NULLIF(?,0), conversation_id),
		finished_at=IF(? IN ('success','failed','skipped','stopped','interrupted'), NOW(), finished_at)
		WHERE id=?`, status, out, errText, convID, status, id)
	return err
}

// SetNodeExecutionEvents 覆盖写入实时事件列表。
func (s *Store) SetNodeExecutionEvents(ctx context.Context, id int64, eventsJSON string) error {
	if strings.TrimSpace(eventsJSON) == "" {
		eventsJSON = "[]"
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE node_executions SET events_json=CAST(? AS JSON) WHERE id=?`, eventsJSON, id)
	return err
}

// ListNodeExecutions 某 run 全部 attempt。
func (s *Store) ListNodeExecutions(ctx context.Context, runID int64) ([]NodeExecution, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, run_id, node_id, attempt, node_type, agent_id, status, conversation_id,
		       CAST(input_json AS CHAR), CAST(output_json AS CHAR), CAST(events_json AS CHAR),
		       COALESCE(error_text,''), COALESCE(parent_attempt_id,0),
		       started_at, finished_at, created_at
		FROM node_executions WHERE run_id=? ORDER BY id ASC`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []NodeExecution
	for rows.Next() {
		var ne NodeExecution
		var started, finished sql.NullTime
		var inJ, outJ, evJ sql.NullString
		if err := rows.Scan(&ne.ID, &ne.RunID, &ne.NodeID, &ne.Attempt, &ne.NodeType, &ne.AgentID, &ne.Status,
			&ne.ConversationID, &inJ, &outJ, &evJ, &ne.ErrorText, &ne.ParentAttemptID, &started, &finished, &ne.CreatedAt); err != nil {
			return nil, err
		}
		if inJ.Valid {
			ne.InputJSON = inJ.String
		}
		if outJ.Valid {
			ne.OutputJSON = outJ.String
		}
		if evJ.Valid {
			ne.EventsJSON = evJ.String
		}
		if started.Valid {
			t := started.Time
			ne.StartedAt = &t
		}
		if finished.Valid {
			t := finished.Time
			ne.FinishedAt = &t
		}
		out = append(out, ne)
	}
	if out == nil {
		out = []NodeExecution{}
	}
	return out, rows.Err()
}

// LatestNodeExecution 某节点最新 attempt。
func (s *Store) LatestNodeExecution(ctx context.Context, runID int64, nodeID string) (*NodeExecution, error) {
	list, err := s.ListNodeExecutions(ctx, runID)
	if err != nil {
		return nil, err
	}
	var best *NodeExecution
	for i := range list {
		if list[i].NodeID != nodeID {
			continue
		}
		if best == nil || list[i].Attempt > best.Attempt || (list[i].Attempt == best.Attempt && list[i].ID > best.ID) {
			best = &list[i]
		}
	}
	return best, nil
}

// NextNodeAttempt 下一 attempt 序号。
func (s *Store) NextNodeAttempt(ctx context.Context, runID int64, nodeID string) (int, error) {
	var max sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		`SELECT MAX(attempt) FROM node_executions WHERE run_id=? AND node_id=?`, runID, nodeID).Scan(&max)
	if err != nil {
		return 1, err
	}
	if !max.Valid {
		return 1, nil
	}
	return int(max.Int64) + 1, nil
}

// MarkIncompleteRunsWaitingRecovery 服务启动：未完成 run 进入 waiting_recovery（非立刻 interrupted）。
func (s *Store) MarkIncompleteRunsWaitingRecovery(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE workflow_runs SET status='waiting_recovery', finished_at=NULL,
		fail_reason=CONCAT(
			COALESCE(NULLIF(fail_reason,''), ''),
			IF(fail_reason IS NULL OR fail_reason='','',' | '),
			'服务重启，进入恢复等待'
		)
		WHERE status IN ('pending','running','waiting')`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// InterruptIncompleteNodeExecutions 将未完成节点 attempt 标 interrupted，保留已有 output。
func (s *Store) InterruptIncompleteNodeExecutions(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE node_executions SET status='interrupted', finished_at=NOW(),
		error_text=CONCAT(
			COALESCE(NULLIF(error_text,''), ''),
			IF(error_text IS NULL OR error_text='','',' | '),
			'服务重启，节点未完成'
		)
		WHERE status IN ('pending','running','waiting')`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ListWorkflowRunsByStatus 按状态列出运行。
func (s *Store) ListWorkflowRunsByStatus(ctx context.Context, status string, limit int) ([]WorkflowRun, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, definition_id, status, COALESCE(input_prompt,''), fail_node_id, COALESCE(fail_reason,''),
		       started_at, finished_at, created_at
		FROM workflow_runs WHERE status=? ORDER BY id ASC LIMIT ?`, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WorkflowRun
	for rows.Next() {
		var r WorkflowRun
		var started, finished sql.NullTime
		if err := rows.Scan(&r.ID, &r.DefinitionID, &r.Status, &r.InputPrompt, &r.FailNodeID, &r.FailReason,
			&started, &finished, &r.CreatedAt); err != nil {
			return nil, err
		}
		if started.Valid {
			t := started.Time
			r.StartedAt = &t
		}
		if finished.Valid {
			t := finished.Time
			r.FinishedAt = &t
		}
		out = append(out, r)
	}
	if out == nil {
		out = []WorkflowRun{}
	}
	return out, rows.Err()
}

// MarkWaitingRecoveryInterrupted 恢复窗口超时：仍 waiting_recovery → interrupted。
func (s *Store) MarkWaitingRecoveryInterrupted(ctx context.Context, reason string) (int64, error) {
	if strings.TrimSpace(reason) == "" {
		reason = "恢复窗口超时，未能自动续跑"
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE workflow_runs SET status='interrupted', finished_at=NOW(),
		fail_reason=CONCAT(
			COALESCE(NULLIF(fail_reason,''), ''),
			IF(fail_reason IS NULL OR fail_reason='','',' | '),
			?
		)
		WHERE status='waiting_recovery'`, reason)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ListRunningNodeConversationIDs 正在执行的节点会话 ID（用于工作流待授权聚合）。
func (s *Store) ListRunningNodeConversationIDs(ctx context.Context) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT conversation_id FROM node_executions
		WHERE status='running' AND conversation_id>0`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// CountActiveWorkflowRuns 忙碌中的团队运行数（含恢复中）。
func (s *Store) CountActiveWorkflowRuns(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM workflow_runs
		WHERE status IN ('pending','running','waiting','waiting_recovery')`).Scan(&n)
	return n, err
}

// UpdateWorkflowRunPrompt 更新运行输入提示词。
func (s *Store) UpdateWorkflowRunPrompt(ctx context.Context, id int64, prompt string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE workflow_runs SET input_prompt=? WHERE id=?`, prompt, id)
	return err
}

// ParseGraphJSON 解析图。
func ParseGraphJSON(raw string) (map[string]any, error) {
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, err
	}
	return m, nil
}
