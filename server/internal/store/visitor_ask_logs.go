package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Ask 审计状态。
const (
	AskStatusProcessing = "processing"
	AskStatusSuccess    = "success"
	AskStatusError      = "error"
	AskStatusCancelled  = "cancelled"
)

// VisitorAskLog 一轮前台 Ask 审计。
type VisitorAskLog struct {
	ID                 int64      `json:"id"`
	VisitorID          int64      `json:"visitor_id"`
	ConversationID     int64      `json:"conversation_id"`
	AgentID            int64      `json:"agent_id"`
	IP                 string     `json:"ip"`
	UserAgent          string     `json:"user_agent"`
	UserMessageID      int64      `json:"user_message_id"`
	AssistantMessageID int64      `json:"assistant_message_id"`
	InputTokens        int64      `json:"input_tokens"`
	OutputTokens       int64      `json:"output_tokens"`
	CachedTokens       int64      `json:"cached_tokens"`
	TotalTokens        int64      `json:"total_tokens"`
	DurationMs         int        `json:"duration_ms"`
	Status             string     `json:"status"`
	ErrorMessage       string     `json:"error_message,omitempty"`
	FinishedAt         *time.Time `json:"finished_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
}

// VisitorAskLogListItem 列表行（JOIN 访客/Agent/用户问题）。
type VisitorAskLogListItem struct {
	ID           int64     `json:"id"`
	VisitorID    int64     `json:"visitorId"`
	VisitorName  string    `json:"visitorName"`
	ConversationID int64   `json:"conversationId"`
	AgentID      int64     `json:"agentId"`
	AgentName    string    `json:"agentName"`
	Question     string    `json:"question"`
	InputTokens  int64     `json:"inputTokens"`
	OutputTokens int64     `json:"outputTokens"`
	CachedTokens int64     `json:"cachedTokens"`
	TotalTokens  int64     `json:"totalTokens"`
	DurationMs   int       `json:"durationMs"`
	Status       string    `json:"status"`
	IP           string    `json:"ip"`
	CreatedAt    time.Time `json:"createdAt"`
}

// VisitorAskLogDetail 审计详情。
type VisitorAskLogDetail struct {
	VisitorAskLog
	VisitorName    string `json:"visitor_name"`
	VisitorPublicID string `json:"visitor_public_id"`
	AgentName      string `json:"agent_name"`
	AgentEngine    string `json:"agent_engine"`
	Question       string `json:"question"`
	Answer         string `json:"answer"`
}

// VisitorAskLogFilter 列表筛选。
type VisitorAskLogFilter struct {
	Page        int
	PageSize    int
	VisitorName string
	AgentID     int64
	Status      string
	Keyword     string
	BeginTime   string // "2006-01-02 15:04:05" 或日期
	EndTime     string
}

// ensureVisitorAskLogAuditColumns 幂等补齐审计扩展列与索引。
func (s *Store) ensureVisitorAskLogAuditColumns(ctx context.Context) error {
	cols := []struct{ name, ddl string }{
		{"user_message_id", `ALTER TABLE visitor_ask_logs ADD COLUMN user_message_id BIGINT NOT NULL DEFAULT 0 AFTER user_agent`},
		{"assistant_message_id", `ALTER TABLE visitor_ask_logs ADD COLUMN assistant_message_id BIGINT NOT NULL DEFAULT 0 AFTER user_message_id`},
		{"input_tokens", `ALTER TABLE visitor_ask_logs ADD COLUMN input_tokens BIGINT NOT NULL DEFAULT 0 AFTER assistant_message_id`},
		{"output_tokens", `ALTER TABLE visitor_ask_logs ADD COLUMN output_tokens BIGINT NOT NULL DEFAULT 0 AFTER input_tokens`},
		{"cached_tokens", `ALTER TABLE visitor_ask_logs ADD COLUMN cached_tokens BIGINT NOT NULL DEFAULT 0 AFTER output_tokens`},
		{"total_tokens", `ALTER TABLE visitor_ask_logs ADD COLUMN total_tokens BIGINT NOT NULL DEFAULT 0 AFTER cached_tokens`},
		{"duration_ms", `ALTER TABLE visitor_ask_logs ADD COLUMN duration_ms INT NOT NULL DEFAULT 0 AFTER total_tokens`},
		{"status", `ALTER TABLE visitor_ask_logs ADD COLUMN status VARCHAR(20) NOT NULL DEFAULT 'processing' AFTER duration_ms`},
		{"error_message", `ALTER TABLE visitor_ask_logs ADD COLUMN error_message TEXT NULL AFTER status`},
		{"finished_at", `ALTER TABLE visitor_ask_logs ADD COLUMN finished_at DATETIME NULL AFTER error_message`},
	}
	for _, col := range cols {
		var n int
		if err := s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM information_schema.COLUMNS
			 WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'visitor_ask_logs' AND COLUMN_NAME = ?`, col.name,
		).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			if _, err := s.db.ExecContext(ctx, col.ddl); err != nil {
				return err
			}
		}
	}
	indexes := []struct{ name, ddl string }{
		{"idx_ask_user_message", `ALTER TABLE visitor_ask_logs ADD KEY idx_ask_user_message (user_message_id)`},
		{"idx_ask_assistant_message", `ALTER TABLE visitor_ask_logs ADD KEY idx_ask_assistant_message (assistant_message_id)`},
		{"idx_ask_conversation_time", `ALTER TABLE visitor_ask_logs ADD KEY idx_ask_conversation_time (conversation_id, created_at)`},
		{"idx_ask_visitor_time", `ALTER TABLE visitor_ask_logs ADD KEY idx_ask_visitor_time (visitor_id, created_at)`},
		{"idx_ask_status_time", `ALTER TABLE visitor_ask_logs ADD KEY idx_ask_status_time (status, created_at)`},
	}
	for _, idx := range indexes {
		var n int
		if err := s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM information_schema.STATISTICS
			 WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'visitor_ask_logs' AND INDEX_NAME = ?`, idx.name,
		).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			if _, err := s.db.ExecContext(ctx, idx.ddl); err != nil {
				return err
			}
		}
	}
	return nil
}

// BeginVisitorAskLog 用户消息落库后创建 processing 审计行。
func (s *Store) BeginVisitorAskLog(ctx context.Context, visitorID, convID, agentID, userMsgID int64, ip, ua string) (int64, error) {
	if len(ua) > 512 {
		ua = ua[:512]
	}
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO visitor_ask_logs
		  (visitor_id, conversation_id, agent_id, ip, user_agent, user_message_id, status)
		VALUES (?,?,?,?,?,?,?)`,
		visitorID, convID, agentID, ip, ua, userMsgID, AskStatusProcessing)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// FinishVisitorAskLog 结束一轮审计（成功/失败/取消）。
func (s *Store) FinishVisitorAskLog(ctx context.Context, id int64, assistantMsgID, input, output, cached, total int64, durationMs int, status, errMsg string) error {
	if id <= 0 {
		return nil
	}
	status = strings.TrimSpace(status)
	if status == "" {
		status = AskStatusSuccess
	}
	if len(errMsg) > 4000 {
		errMsg = errMsg[:4000]
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE visitor_ask_logs SET
		  assistant_message_id = CASE WHEN ? > 0 THEN ? ELSE assistant_message_id END,
		  input_tokens = ?, output_tokens = ?, cached_tokens = ?, total_tokens = ?,
		  duration_ms = ?, status = ?, error_message = NULLIF(?, ''), finished_at = NOW()
		WHERE id = ?`,
		assistantMsgID, assistantMsgID, input, output, cached, total, durationMs, status, errMsg, id)
	return err
}

// GetVisitorAskLog 单条原始审计。
func (s *Store) GetVisitorAskLog(ctx context.Context, id int64) (*VisitorAskLog, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, visitor_id, conversation_id, agent_id, IFNULL(ip,''), IFNULL(user_agent,''),
		       IFNULL(user_message_id,0), IFNULL(assistant_message_id,0),
		       IFNULL(input_tokens,0), IFNULL(output_tokens,0), IFNULL(cached_tokens,0), IFNULL(total_tokens,0),
		       IFNULL(duration_ms,0), IFNULL(status,'processing'), IFNULL(error_message,''),
		       finished_at, created_at
		FROM visitor_ask_logs WHERE id=?`, id)
	var log VisitorAskLog
	var finished sql.NullTime
	err := row.Scan(
		&log.ID, &log.VisitorID, &log.ConversationID, &log.AgentID, &log.IP, &log.UserAgent,
		&log.UserMessageID, &log.AssistantMessageID,
		&log.InputTokens, &log.OutputTokens, &log.CachedTokens, &log.TotalTokens,
		&log.DurationMs, &log.Status, &log.ErrorMessage, &finished, &log.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if finished.Valid {
		t := finished.Time
		log.FinishedAt = &t
	}
	return &log, nil
}

// GetVisitorAskLogDetail 审计详情（含消息正文）。
func (s *Store) GetVisitorAskLogDetail(ctx context.Context, id int64) (*VisitorAskLogDetail, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT a.id, a.visitor_id, a.conversation_id, a.agent_id, IFNULL(a.ip,''), IFNULL(a.user_agent,''),
		       IFNULL(a.user_message_id,0), IFNULL(a.assistant_message_id,0),
		       IFNULL(a.input_tokens,0), IFNULL(a.output_tokens,0), IFNULL(a.cached_tokens,0), IFNULL(a.total_tokens,0),
		       IFNULL(a.duration_ms,0), IFNULL(a.status,'processing'), IFNULL(a.error_message,''),
		       a.finished_at, a.created_at,
		       IFNULL(v.name,''), IFNULL(v.public_id,''),
		       IFNULL(ag.name,''), IFNULL(ag.engine,''),
		       IFNULL(um.content,''), IFNULL(am.content,'')
		FROM visitor_ask_logs a
		LEFT JOIN visitors v ON v.id = a.visitor_id
		LEFT JOIN managed_agents ag ON ag.id = a.agent_id
		LEFT JOIN messages um ON um.id = a.user_message_id
		LEFT JOIN messages am ON am.id = a.assistant_message_id
		WHERE a.id = ?`, id)
	var d VisitorAskLogDetail
	var finished sql.NullTime
	err := row.Scan(
		&d.ID, &d.VisitorID, &d.ConversationID, &d.AgentID, &d.IP, &d.UserAgent,
		&d.UserMessageID, &d.AssistantMessageID,
		&d.InputTokens, &d.OutputTokens, &d.CachedTokens, &d.TotalTokens,
		&d.DurationMs, &d.Status, &d.ErrorMessage, &finished, &d.CreatedAt,
		&d.VisitorName, &d.VisitorPublicID, &d.AgentName, &d.AgentEngine,
		&d.Question, &d.Answer,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if finished.Valid {
		t := finished.Time
		d.FinishedAt = &t
	}
	return &d, nil
}

// ListVisitorAskLogs 分页列表。
func (s *Store) ListVisitorAskLogs(ctx context.Context, f VisitorAskLogFilter) ([]VisitorAskLogListItem, int, error) {
	if f.Page <= 0 {
		f.Page = 1
	}
	if f.PageSize <= 0 || f.PageSize > 100 {
		f.PageSize = 20
	}
	where := []string{"1=1"}
	args := []any{}
	if name := strings.TrimSpace(f.VisitorName); name != "" {
		where = append(where, "v.name LIKE ?")
		args = append(args, "%"+name+"%")
	}
	if f.AgentID > 0 {
		where = append(where, "a.agent_id = ?")
		args = append(args, f.AgentID)
	}
	if st := strings.TrimSpace(f.Status); st != "" {
		where = append(where, "a.status = ?")
		args = append(args, st)
	}
	if kw := strings.TrimSpace(f.Keyword); kw != "" {
		where = append(where, "um.content LIKE ?")
		args = append(args, "%"+kw+"%")
	}
	if bt := strings.TrimSpace(f.BeginTime); bt != "" {
		where = append(where, "a.created_at >= ?")
		args = append(args, bt)
	}
	if et := strings.TrimSpace(f.EndTime); et != "" {
		where = append(where, "a.created_at <= ?")
		args = append(args, et)
	}
	w := strings.Join(where, " AND ")
	from := `
		FROM visitor_ask_logs a
		LEFT JOIN visitors v ON v.id = a.visitor_id
		LEFT JOIN managed_agents ag ON ag.id = a.agent_id
		LEFT JOIN messages um ON um.id = a.user_message_id`

	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) `+from+` WHERE `+w, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	offset := (f.Page - 1) * f.PageSize
	q := `
		SELECT a.id, a.visitor_id, IFNULL(v.name,''), a.conversation_id, a.agent_id, IFNULL(ag.name,''),
		       IFNULL(um.content,''),
		       IFNULL(a.input_tokens,0), IFNULL(a.output_tokens,0), IFNULL(a.cached_tokens,0), IFNULL(a.total_tokens,0),
		       IFNULL(a.duration_ms,0), IFNULL(a.status,'processing'), IFNULL(a.ip,''), a.created_at
		` + from + ` WHERE ` + w + `
		ORDER BY a.created_at DESC, a.id DESC
		LIMIT ? OFFSET ?`
	args2 := append(append([]any{}, args...), f.PageSize, offset)
	rows, err := s.db.QueryContext(ctx, q, args2...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []VisitorAskLogListItem
	for rows.Next() {
		var it VisitorAskLogListItem
		if err := rows.Scan(
			&it.ID, &it.VisitorID, &it.VisitorName, &it.ConversationID, &it.AgentID, &it.AgentName,
			&it.Question, &it.InputTokens, &it.OutputTokens, &it.CachedTokens, &it.TotalTokens,
			&it.DurationMs, &it.Status, &it.IP, &it.CreatedAt,
		); err != nil {
			return nil, 0, err
		}
		out = append(out, it)
	}
	if out == nil {
		out = []VisitorAskLogListItem{}
	}
	return out, total, rows.Err()
}

// InsertVisitorAskLog 兼容旧调用：仅写基础行（无 message_id）。新路径请用 BeginVisitorAskLog。
func (s *Store) InsertVisitorAskLog(ctx context.Context, visitorID, convID, agentID int64, ip, ua string) error {
	_, err := s.BeginVisitorAskLog(ctx, visitorID, convID, agentID, 0, ip, ua)
	return err
}

// MaskIP 列表脱敏：保留首段与末段。
func MaskIP(ip string) string {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return ""
	}
	parts := strings.Split(ip, ".")
	if len(parts) == 4 {
		return fmt.Sprintf("%s.%s.***.%s", parts[0], parts[1], parts[3])
	}
	// IPv6 / 其他：中间打码
	if len(ip) <= 8 {
		return ip
	}
	return ip[:4] + "****" + ip[len(ip)-4:]
}
