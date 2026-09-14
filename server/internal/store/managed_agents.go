package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

// ManagedAgent 管理台注册的智能体。
type ManagedAgent struct {
	ID              int64      `json:"id"`
	Name            string     `json:"name"`
	FolderID        int64      `json:"folder_id"` // 0=根目录
	Engine          string     `json:"engine"`    // claude | codex | agent
	BinPath         string     `json:"bin_path"`
	RulesPrompt     string     `json:"rules_prompt"`
	AutoReview      bool       `json:"auto_review"`
	AllowWrite      bool       `json:"allow_write"`
	AllowNetwork    bool       `json:"allow_network"`
	AllowRm         bool       `json:"allow_rm"`
	AllowBrowser    bool       `json:"allow_browser"`
	WorkspacePath   string     `json:"workspace_path"`
	ScheduleEnabled bool       `json:"schedule_enabled"`
	ScheduleCron    string     `json:"schedule_cron"`
	ScheduleLabel   string     `json:"schedule_label"`
	Status          string     `json:"status"`
	ConversationID  int64      `json:"conversation_id"`
	LastError       string     `json:"last_error"`
	LastRunMs       int        `json:"last_run_ms"`
	RunStartedAt    *time.Time `json:"run_started_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// ManagedAgentInput 创建/更新智能体入参。
type ManagedAgentInput struct {
	Name            string
	Engine          string
	BinPath         string
	RulesPrompt     string
	AllowWrite      *bool
	AllowNetwork    *bool
	AllowRm         *bool
	AllowBrowser    *bool
	WorkspacePath   *string
	ScheduleEnabled *bool
	ScheduleCron    *string
	ScheduleLabel   *string
}

// ValidEngine 校验引擎类型。
func ValidEngine(e string) bool {
	switch strings.ToLower(strings.TrimSpace(e)) {
	case "claude", "codex", "agent":
		return true
	default:
		return false
	}
}

// DefaultBin 返回引擎默认二进制名。
func DefaultBin(engine string) string {
	switch strings.ToLower(engine) {
	case "claude":
		return "claude"
	case "codex":
		return "codex"
	case "agent":
		return "agent"
	default:
		return ""
	}
}

func bool01(b bool) int {
	if b {
		return 1
	}
	return 0
}

// scanManagedAgent 扫描一行 managed_agents。
func scanManagedAgent(scanner interface {
	Scan(dest ...any) error
}) (ManagedAgent, error) {
	var a ManagedAgent
	var runAt sql.NullTime
	var auto, aw, an, ar, ab, se int
	err := scanner.Scan(
		&a.ID, &a.Name, &a.FolderID, &a.Engine, &a.BinPath, &a.RulesPrompt, &auto,
		&aw, &an, &ar, &ab, &a.WorkspacePath, &se, &a.ScheduleCron, &a.ScheduleLabel,
		&a.Status, &a.ConversationID, &a.LastError, &a.LastRunMs, &runAt, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return a, err
	}
	a.AutoReview = auto != 0
	a.AllowWrite = aw != 0
	a.AllowNetwork = an != 0
	a.AllowRm = ar != 0
	a.AllowBrowser = ab != 0
	a.ScheduleEnabled = se != 0
	if runAt.Valid {
		t := runAt.Time
		a.RunStartedAt = &t
	}
	return a, nil
}

const managedSelectCols = `id, name, IFNULL(folder_id,0), engine, bin_path, IFNULL(rules_prompt,''), IFNULL(auto_review,0),
		IFNULL(allow_write,1), IFNULL(allow_network,1), IFNULL(allow_rm,0), IFNULL(allow_browser,0), IFNULL(workspace_path,''),
		IFNULL(schedule_enabled,0), IFNULL(schedule_cron,''), IFNULL(schedule_label,''),
		status, IFNULL(conversation_id,0), IFNULL(last_error,''), IFNULL(last_run_ms,0),
		run_started_at, created_at, updated_at`

// ListManagedAgents 全部智能体(最新更新在前)。
func (s *Store) ListManagedAgents(ctx context.Context) ([]ManagedAgent, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+managedSelectCols+` FROM managed_agents ORDER BY updated_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ManagedAgent
	for rows.Next() {
		a, err := scanManagedAgent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ListScheduledAgents 已开启定时的智能体。
func (s *Store) ListScheduledAgents(ctx context.Context) ([]ManagedAgent, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+managedSelectCols+` FROM managed_agents
		 WHERE IFNULL(schedule_enabled,0)=1 AND IFNULL(schedule_cron,'')<>''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ManagedAgent
	for rows.Next() {
		a, err := scanManagedAgent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetManagedAgent 按 id 取一条。
func (s *Store) GetManagedAgent(ctx context.Context, id int64) (*ManagedAgent, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+managedSelectCols+` FROM managed_agents WHERE id=?`, id)
	a, err := scanManagedAgent(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// CreateManagedAgent 创建智能体(含权限策略)。
func (s *Store) CreateManagedAgent(ctx context.Context, in ManagedAgentInput) (int64, error) {
	engine := strings.ToLower(strings.TrimSpace(in.Engine))
	binPath := strings.TrimSpace(in.BinPath)
	if binPath == "" {
		binPath = DefaultBin(engine)
	}
	aw, an, ar, ab := true, true, false, false
	if in.AllowWrite != nil {
		aw = *in.AllowWrite
	}
	if in.AllowNetwork != nil {
		an = *in.AllowNetwork
	}
	if in.AllowRm != nil {
		ar = *in.AllowRm
	}
	if in.AllowBrowser != nil {
		ab = *in.AllowBrowser
	}
	ws, cron, label := "", "", ""
	se := false
	if in.WorkspacePath != nil {
		ws = strings.TrimSpace(*in.WorkspacePath)
	}
	if in.ScheduleEnabled != nil {
		se = *in.ScheduleEnabled
	}
	if in.ScheduleCron != nil {
		cron = strings.TrimSpace(*in.ScheduleCron)
	}
	if in.ScheduleLabel != nil {
		label = strings.TrimSpace(*in.ScheduleLabel)
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO managed_agents
		 (name, engine, bin_path, rules_prompt, status, allow_write, allow_network, allow_rm, allow_browser,
		  workspace_path, schedule_enabled, schedule_cron, schedule_label)
		 VALUES (?,?,?,?, 'idle',?,?,?,?,?,?,?,?)`,
		in.Name, engine, binPath, in.RulesPrompt,
		bool01(aw), bool01(an), bool01(ar), bool01(ab), ws, bool01(se), cron, label)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateManagedAgent 更新智能体;in 中指针字段非 nil 才覆盖,Name/BinPath/RulesPrompt 用 Has* 由 handler 填满后直接写。
// 约定:handler 保存设置时传入完整合并后的 Name/BinPath/RulesPrompt 与全部策略指针。
func (s *Store) UpdateManagedAgent(ctx context.Context, id int64, in ManagedAgentInput) error {
	a, err := s.GetManagedAgent(ctx, id)
	if err != nil || a == nil {
		if a == nil && err == nil {
			return sql.ErrNoRows
		}
		return err
	}
	n, b, r := a.Name, a.BinPath, a.RulesPrompt
	aw, an, ar, ab := a.AllowWrite, a.AllowNetwork, a.AllowRm, a.AllowBrowser
	ws, se, cron, label := a.WorkspacePath, a.ScheduleEnabled, a.ScheduleCron, a.ScheduleLabel

	if strings.TrimSpace(in.Name) != "" {
		n = strings.TrimSpace(in.Name)
	}
	if strings.TrimSpace(in.BinPath) != "" {
		b = strings.TrimSpace(in.BinPath)
	}
	// RulesPrompt: 用策略指针非空作为「完整设置保存」标记,允许清空规则
	if in.AllowWrite != nil || in.AllowNetwork != nil || in.AllowRm != nil || in.AllowBrowser != nil || in.WorkspacePath != nil || in.ScheduleEnabled != nil {
		r = in.RulesPrompt
	} else if in.RulesPrompt != "" {
		r = in.RulesPrompt
	}
	if in.AllowWrite != nil {
		aw = *in.AllowWrite
	}
	if in.AllowNetwork != nil {
		an = *in.AllowNetwork
	}
	if in.AllowRm != nil {
		ar = *in.AllowRm
	}
	if in.AllowBrowser != nil {
		ab = *in.AllowBrowser
	}
	if in.WorkspacePath != nil {
		ws = strings.TrimSpace(*in.WorkspacePath)
	}
	if in.ScheduleEnabled != nil {
		se = *in.ScheduleEnabled
	}
	if in.ScheduleCron != nil {
		cron = strings.TrimSpace(*in.ScheduleCron)
	}
	if in.ScheduleLabel != nil {
		label = strings.TrimSpace(*in.ScheduleLabel)
	}

	_, err = s.db.ExecContext(ctx,
		`UPDATE managed_agents SET name=?, bin_path=?, rules_prompt=?,
		 allow_write=?, allow_network=?, allow_rm=?, allow_browser=?, workspace_path=?,
		 schedule_enabled=?, schedule_cron=?, schedule_label=? WHERE id=?`,
		n, b, r, bool01(aw), bool01(an), bool01(ar), bool01(ab), ws, bool01(se), cron, label, id)
	return err
}

// SetManagedAgentAutoReview 持久化 AI 自动审核开关。
func (s *Store) SetManagedAgentAutoReview(ctx context.Context, id int64, enabled bool) error {
	res, err := s.db.ExecContext(ctx, `UPDATE managed_agents SET auto_review=? WHERE id=?`, bool01(enabled), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// CountUserMessages 会话中 user 消息数。
func (s *Store) CountUserMessages(ctx context.Context, conversationID int64) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM messages WHERE conversation_id=? AND role='user'`, conversationID).Scan(&n)
	return n, err
}

// DeleteManagedAgent 删除智能体及其会话消息。
func (s *Store) DeleteManagedAgent(ctx context.Context, id int64) error {
	a, err := s.GetManagedAgent(ctx, id)
	if err != nil {
		return err
	}
	if a == nil {
		return sql.ErrNoRows
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if a.ConversationID > 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM conversations WHERE id=?`, a.ConversationID); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM conversations WHERE agent_id=?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM managed_agents WHERE id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// SetManagedAgentStatus 更新运行状态;running 时记录 run_started_at,结束时清空。
func (s *Store) SetManagedAgentStatus(ctx context.Context, id int64, status, lastErr string, runMs int, convID int64) error {
	if status == "running" {
		_, err := s.db.ExecContext(ctx,
			`UPDATE managed_agents SET status=?, last_error=?, last_run_ms=?, run_started_at=NOW(),
			 conversation_id=IFNULL(NULLIF(?,0), conversation_id) WHERE id=?`,
			status, lastErr, runMs, convID, id)
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE managed_agents SET status=?, last_error=?, last_run_ms=?, run_started_at=NULL,
		 conversation_id=IFNULL(NULLIF(?,0), conversation_id) WHERE id=?`,
		status, lastErr, runMs, convID, id)
	return err
}

// SetManagedAgentConversation 绑定当前会话。
func (s *Store) SetManagedAgentConversation(ctx context.Context, id, convID int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE managed_agents SET conversation_id=? WHERE id=?`, convID, id)
	return err
}

// CreateAgentConversation 为管理型智能体新建 chat 会话。
func (s *Store) CreateAgentConversation(ctx context.Context, agentID int64) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO conversations (user_ip, mode, agent_id) VALUES (?,?,?)`,
		"admin", "chat", agentID)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ClearConversationMessages 清空会话全部消息。
func (s *Store) ClearConversationMessages(ctx context.Context, conversationID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM messages WHERE conversation_id=?`, conversationID)
	return err
}

// AgentTasksByAgentID 按 agent_id 过滤任务(可空 agentType)。
func (s *Store) AgentTasksByAgentID(ctx context.Context, agentType, agentID, status string) ([]AgentTask, error) {
	q := `SELECT id, agent_type, agent_id, task_id, task_name, status, progress, IFNULL(message,''), IFNULL(payload,'null'), created_at, updated_at
	      FROM agent_tasks WHERE 1=1`
	args := []interface{}{}
	if agentType != "" {
		q += ` AND agent_type=?`
		args = append(args, agentType)
	}
	if agentID != "" {
		q += ` AND agent_id=?`
		args = append(args, agentID)
	}
	if status != "" {
		q += ` AND status=?`
		args = append(args, status)
	}
	q += ` ORDER BY updated_at DESC LIMIT 200`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AgentTask
	for rows.Next() {
		var t AgentTask
		var payload []byte
		if err := rows.Scan(&t.ID, &t.AgentType, &t.AgentID, &t.TaskID, &t.TaskName, &t.Status, &t.Progress, &t.Message, &payload, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		if len(payload) > 0 {
			_ = json.Unmarshal(payload, &t.Payload)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
