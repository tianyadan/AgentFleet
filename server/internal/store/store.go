package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// Conversation 一个会话(按 user_ip 分组)。
type Conversation struct {
	ID        int64     `json:"id"`
	UserIP    string    `json:"user_ip"`
	Mode      string    `json:"mode"` // single / chat
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Message 一条消息。
type Message struct {
	ID                  int64     `json:"id"`
	ConversationID      int64     `json:"conversation_id"`
	Role                string    `json:"role"` // user / assistant / invoke_quote / invoke_divider
	Content             string    `json:"content"`
	Status              string    `json:"status"`
	WorkspacePath       string    `json:"workspace_path"`
	ReplyMs             int       `json:"reply_ms"`
	ExcludeFromContext  bool      `json:"exclude_from_context"`
	MetaJSON            string    `json:"meta_json,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
}

// Workspace 一条授权工作区记录。
type Workspace struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Path    string `json:"path"`
	Type    string `json:"type"` // code / db
	Enabled bool   `json:"enabled"`
}

// Store 封装 MySQL 访问。
type Store struct {
	db *sql.DB
}

// New 建立连接池。
func New(dsn string) (*Store, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	if err := db.Ping(); err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.Migrate(context.Background()); err != nil {
		return nil, err
	}
	return s, nil
}

// Migrate 幂等地把 authorized_workspaces 升级到 v0.0.4 结构:
// 补 type 列(缺省 code)、回填旧行、并种子「数据查询」db 项目。
// 不依赖手工执行 schema.sql,对旧库(v0.0.3)与新库均可安全运行。
func (s *Store) Migrate(ctx context.Context) error {
	// 1) type 列是否存在
	var cnt int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM information_schema.COLUMNS
		 WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'authorized_workspaces' AND COLUMN_NAME = 'type'`,
	).Scan(&cnt); err != nil {
		return err
	}
	if cnt == 0 {
		if _, err := s.db.ExecContext(ctx,
			`ALTER TABLE authorized_workspaces ADD COLUMN type VARCHAR(16) NOT NULL DEFAULT 'code' AFTER path`,
		); err != nil {
			return err
		}
	}
	// 2) 回填历史行的 type(仅 code 目录,不含 db)
	if _, err := s.db.ExecContext(ctx,
		`UPDATE authorized_workspaces SET type='code' WHERE type IS NULL OR type=''`,
	); err != nil {
		return err
	}
	// 3) 种子「数据查询」db 项目(并入授权工作区)
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO authorized_workspaces (name, path, type, enabled)
		 VALUES ('数据查询', 'db://data-query', 'db', 1)
		 ON DUPLICATE KEY UPDATE enabled = VALUES(enabled), type = VALUES(type)`,
	); err != nil {
		return err
	}
	// 4) v0.2.14 智能体文件夹
	if _, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS agent_folders (
		  id         BIGINT       NOT NULL AUTO_INCREMENT,
		  name       VARCHAR(128) NOT NULL,
		  sort_order INT          NOT NULL DEFAULT 0,
		  created_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
		  PRIMARY KEY (id)
		) ENGINE=InnoDB`); err != nil {
		return err
	}
	var folderCol int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM information_schema.COLUMNS
		 WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'managed_agents' AND COLUMN_NAME = 'folder_id'`,
	).Scan(&folderCol); err != nil {
		return err
	}
	if folderCol == 0 {
		if _, err := s.db.ExecContext(ctx,
			`ALTER TABLE managed_agents ADD COLUMN folder_id BIGINT NULL AFTER name`,
		); err != nil {
			return err
		}
	}
	// 5) v0.2.15 引擎会话与上下文快照
	for _, col := range []struct{ name, ddl string }{
		{"engine_session_id", `ALTER TABLE conversations ADD COLUMN engine_session_id VARCHAR(128) NULL`},
		{"engine_used_tokens", `ALTER TABLE conversations ADD COLUMN engine_used_tokens BIGINT NOT NULL DEFAULT 0`},
		{"engine_window_tokens", `ALTER TABLE conversations ADD COLUMN engine_window_tokens BIGINT NOT NULL DEFAULT 0`},
	} {
		var n int
		if err := s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM information_schema.COLUMNS
			 WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'conversations' AND COLUMN_NAME = ?`, col.name,
		).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			if _, err := s.db.ExecContext(ctx, col.ddl); err != nil {
				return err
			}
		}
	}
	// 6) v0.2.17：一次性清除曾被任务拆分污染的 Claude engine_session
	if _, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_patches (
		  id         VARCHAR(64) NOT NULL,
		  applied_at DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
		  PRIMARY KEY (id)
		) ENGINE=InnoDB`); err != nil {
		return err
	}
	var patched int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM schema_patches WHERE id='v0.2.17-clear-claude-plan-session'`,
	).Scan(&patched); err != nil {
		return err
	}
	if patched == 0 {
		if _, err := s.db.ExecContext(ctx, `
			UPDATE conversations c
			INNER JOIN managed_agents a ON a.conversation_id = c.id
			SET c.engine_session_id = NULL
			WHERE LOWER(IFNULL(a.engine, '')) = 'claude'
			  AND c.engine_session_id IS NOT NULL
			  AND TRIM(c.engine_session_id) <> ''`); err != nil {
			return err
		}
		if _, err := s.db.ExecContext(ctx,
			`INSERT INTO schema_patches (id) VALUES ('v0.2.17-clear-claude-plan-session')`); err != nil {
			return err
		}
	}
	return nil
}

// Workspaces 返回启用的授权工作区。type 缺省值为 code(兼容旧表)。
func (s *Store) Workspaces(ctx context.Context) ([]Workspace, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, path, COALESCE(type,'code'), enabled FROM authorized_workspaces WHERE enabled = 1 ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Workspace
	for rows.Next() {
		var w Workspace
		if err := rows.Scan(&w.ID, &w.Name, &w.Path, &w.Type, &w.Enabled); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// CodeWorkspaces 仅返回 code 类型的授权工作区(用于文件系统访问授权)。type 缺省视为 code。
func (s *Store) CodeWorkspaces(ctx context.Context) ([]Workspace, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, path, COALESCE(type,'code'), enabled FROM authorized_workspaces WHERE enabled = 1 AND COALESCE(type,'code')='code' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Workspace
	for rows.Next() {
		var w Workspace
		if err := rows.Scan(&w.ID, &w.Name, &w.Path, &w.Type, &w.Enabled); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// ---- conversations ----

// CreateConversation 新建会话。
func (s *Store) CreateConversation(ctx context.Context, userIP, mode string) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO conversations (user_ip, mode) VALUES (?, ?)`, userIP, mode)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ConvWithPreview 一条会话及其最近一条用户消息预览。
type ConvWithPreview struct {
	Conversation
	Preview string `json:"preview"`
}

// TrimIPConversations 保留该 IP 最新的 limit 个会话,更旧的删除(messages 靠外键级联)。
func (s *Store) TrimIPConversations(ctx context.Context, ip string, limit int) error {
	if limit <= 0 {
		limit = 20
	}
	_, err := s.db.ExecContext(ctx,
		`DELETE c FROM conversations c
		 WHERE c.user_ip = ?
		   AND c.id NOT IN (
		     SELECT id FROM (
		       SELECT id FROM conversations
		       WHERE user_ip = ?
		       ORDER BY updated_at DESC, id DESC
		       LIMIT ?
		     ) keep
		   )`, ip, ip, limit)
	return err
}

// ConversationsAllIP 返回**全部 IP** 的会话列表(覆盖所有 user_ip),按
// user_ip + 最近活跃倒序排列,附每条会话最近一条用户消息预览。
// 供历史页按 IP 分组;每 IP 的条数裁剪由调用方用 TrimIPConversations 完成。
func (s *Store) ConversationsAllIP(ctx context.Context) ([]ConvWithPreview, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT c.id, c.user_ip, c.mode, c.created_at, c.updated_at,
		        (SELECT mc.content FROM messages mc WHERE mc.conversation_id = c.id AND mc.role='user' ORDER BY mc.id DESC LIMIT 1) AS preview
		 FROM conversations c
		 ORDER BY c.user_ip, c.updated_at DESC, c.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ConvWithPreview
	for rows.Next() {
		var c ConvWithPreview
		var preview sql.NullString
		if err := rows.Scan(&c.ID, &c.UserIP, &c.Mode, &c.CreatedAt, &c.UpdatedAt, &preview); err != nil {
			return nil, err
		}
		if preview.Valid {
			c.Preview = preview.String
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ConversationIPs 返回所有出现过会话的 user_ip,用于逐 IP 裁剪。
func (s *Store) ConversationIPs(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT user_ip FROM conversations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ips []string
	for rows.Next() {
		var ip string
		if err := rows.Scan(&ip); err != nil {
			return nil, err
		}
		ips = append(ips, ip)
	}
	return ips, rows.Err()
}

// ---- 多 Agent 统一管理中心 ----

// AgentTask 一条来自某 agent 的任务进度汇报。
type AgentTask struct {
	ID        int64     `json:"id"`
	AgentType string    `json:"agent_type"`
	AgentID   string    `json:"agent_id"`
	TaskID    string    `json:"task_id"`
	TaskName  string    `json:"task_name"`
	Status    string    `json:"status"`
	Progress  int       `json:"progress"`
	Message   string    `json:"message"`
	Payload   any       `json:"payload,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AgentTaskReport 上报入参(agent 侧调用)。
type AgentTaskReport struct {
	AgentType string `json:"agent_type"`
	AgentID   string `json:"agent_id"`
	TaskID    string `json:"task_id"`
	TaskName  string `json:"task_name"`
	Status    string `json:"status"`
	Progress  int    `json:"progress"`
	Message   string `json:"message"`
	Payload   any    `json:"payload"`
}

// UpsertAgentTask 按 (agent_type, task_id) 幂等 upsert 一条任务进度。
func (s *Store) UpsertAgentTask(ctx context.Context, r AgentTaskReport) (int64, error) {
	var payload any = r.Payload
	if r.Payload != nil {
		if b, err := json.Marshal(r.Payload); err == nil {
			payload = b
		}
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO agent_tasks (agent_type, agent_id, task_id, task_name, status, progress, message, payload)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE
		   agent_id   = VALUES(agent_id),
		   task_name = VALUES(task_name),
		   status    = VALUES(status),
		   progress = VALUES(progress),
		   message  = VALUES(message),
		   payload  = VALUES(payload)`,
		r.AgentType, r.AgentID, r.TaskID, r.TaskName, r.Status, r.Progress, r.Message, payload)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// AgentTasks 返回任务列表。agentType/status 为空表示不过滤,按最近更新倒序。
func (s *Store) AgentTasks(ctx context.Context, agentType, status string) ([]AgentTask, error) {
	q := `SELECT id, agent_type, agent_id, task_id, task_name, status, progress, message, payload, created_at, updated_at
	      FROM agent_tasks WHERE 1=1`
	args := []any{}
	if agentType != "" {
		q += ` AND agent_type = ?`
		args = append(args, agentType)
	}
	if status != "" {
		q += ` AND status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY updated_at DESC`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AgentTask
	for rows.Next() {
		var t AgentTask
		var p []byte
		if err := rows.Scan(&t.ID, &t.AgentType, &t.AgentID, &t.TaskID, &t.TaskName,
			&t.Status, &t.Progress, &t.Message, &p, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		if len(p) > 0 {
			_ = json.Unmarshal(p, &t.Payload)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// AgentTaskTypes 返回出现过的 agent_type(去重),用于可插拔分组。
func (s *Store) AgentTaskTypes(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT agent_type FROM agent_tasks ORDER BY agent_type`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ConversationOwner 返回会话归属的 user_ip(不存在返回空串)。
func (s *Store) ConversationOwner(ctx context.Context, id int64) (string, error) {
	var ip string
	err := s.db.QueryRowContext(ctx, `SELECT user_ip FROM conversations WHERE id = ?`, id).Scan(&ip)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return ip, err
}

// ConversationExists 校验会话存在且属于该 IP。
func (s *Store) ConversationExists(ctx context.Context, id int64, userIP string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM conversations WHERE id = ? AND user_ip = ?`, id, userIP).Scan(&n)
	return n > 0, err
}

// ---- messages ----

// InsertMessage 写入一条消息。
func (s *Store) InsertMessage(ctx context.Context, m *Message) (int64, error) {
	ex := 0
	if m.ExcludeFromContext {
		ex = 1
	}
	meta := m.MetaJSON
	if meta == "" {
		meta = "null"
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO messages (conversation_id, role, content, status, workspace_path, reply_ms, exclude_from_context, meta_json)
		 VALUES (?, ?, ?, ?, ?, ?, ?, CAST(? AS JSON))`,
		m.ConversationID, m.Role, m.Content, m.Status, m.WorkspacePath, m.ReplyMs, ex, meta)
	if err != nil {
		// 兼容未迁移库:回退旧列
		res, err = s.db.ExecContext(ctx,
			`INSERT INTO messages (conversation_id, role, content, status, workspace_path, reply_ms)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			m.ConversationID, m.Role, m.Content, m.Status, m.WorkspacePath, m.ReplyMs)
		if err != nil {
			return 0, err
		}
	}
	// 更新会话活跃时间
	s.db.ExecContext(ctx, `UPDATE conversations SET updated_at = NOW() WHERE id = ?`, m.ConversationID)
	id, _ := res.LastInsertId()
	m.ID = id
	return id, nil
}

// Messages 返回某会话的全部消息(按创建顺序)。
func (s *Store) Messages(ctx context.Context, conversationID int64) ([]Message, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, conversation_id, role, content, status, workspace_path, reply_ms,
		        IFNULL(exclude_from_context,0), IFNULL(CAST(meta_json AS CHAR), ''), created_at
		 FROM messages WHERE conversation_id = ? ORDER BY id`, conversationID)
	if err != nil {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, conversation_id, role, content, status, workspace_path, reply_ms, created_at
			 FROM messages WHERE conversation_id = ? ORDER BY id`, conversationID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []Message
		for rows.Next() {
			var m Message
			if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content,
				&m.Status, &m.WorkspacePath, &m.ReplyMs, &m.CreatedAt); err != nil {
				return nil, err
			}
			out = append(out, m)
		}
		return out, rows.Err()
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		var m Message
		var ex int
		var meta sql.NullString
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content,
			&m.Status, &m.WorkspacePath, &m.ReplyMs, &ex, &meta, &m.CreatedAt); err != nil {
			return nil, err
		}
		m.ExcludeFromContext = ex != 0
		if meta.Valid && meta.String != "null" {
			m.MetaJSON = meta.String
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// MessagesPage 分页取消息:beforeID=0 取最新 limit 条;否则取 id < beforeID 的最近 limit 条。
// 返回按 id 升序的 items、总数、是否还有更早消息。
func (s *Store) MessagesPage(ctx context.Context, conversationID, beforeID int64, limit int) (items []Message, total int, hasMore bool, err error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages WHERE conversation_id=?`, conversationID).Scan(&total)
	if err != nil {
		return nil, 0, false, err
	}
	var rows *sql.Rows
	if beforeID > 0 {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, conversation_id, role, content, status, workspace_path, reply_ms,
			        IFNULL(exclude_from_context,0), IFNULL(CAST(meta_json AS CHAR), ''), created_at
			 FROM messages WHERE conversation_id=? AND id < ?
			 ORDER BY id DESC LIMIT ?`, conversationID, beforeID, limit)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, conversation_id, role, content, status, workspace_path, reply_ms,
			        IFNULL(exclude_from_context,0), IFNULL(CAST(meta_json AS CHAR), ''), created_at
			 FROM messages WHERE conversation_id=?
			 ORDER BY id DESC LIMIT ?`, conversationID, limit)
	}
	if err != nil {
		return nil, 0, false, err
	}
	defer rows.Close()
	var tmp []Message
	for rows.Next() {
		var m Message
		var ex int
		var meta sql.NullString
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content,
			&m.Status, &m.WorkspacePath, &m.ReplyMs, &ex, &meta, &m.CreatedAt); err != nil {
			return nil, 0, false, err
		}
		m.ExcludeFromContext = ex != 0
		if meta.Valid && meta.String != "null" {
			m.MetaJSON = meta.String
		}
		tmp = append(tmp, m)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, false, err
	}
	// 翻转为时间正序
	for i, j := 0, len(tmp)-1; i < j; i, j = i+1, j-1 {
		tmp[i], tmp[j] = tmp[j], tmp[i]
	}
	items = tmp
	if len(items) == 0 {
		return items, total, false, nil
	}
	oldest := items[0].ID
	var older int
	_ = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM messages WHERE conversation_id=? AND id < ?`, conversationID, oldest).Scan(&older)
	hasMore = older > 0
	return items, total, hasMore, nil
}

// RecentTurns 返回会话最近的 limit 轮(user,assistant)对,用于组装上下文。
type Turn struct {
	User      string
	Assistant string
}

func (s *Store) RecentTurns(ctx context.Context, conversationID int64, limit int) ([]Turn, error) {
	msgs, err := s.Messages(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	// 从后往前,每次取一条 user 及其后续 assistant；排除不进上下文的消息与特殊角色
	var turns []Turn
	for i := len(msgs) - 1; i >= 0 && len(turns) < limit; i-- {
		if msgs[i].ExcludeFromContext {
			continue
		}
		if msgs[i].Role != "user" {
			continue
		}
		asst := ""
		for j := i + 1; j < len(msgs); j++ {
			if msgs[j].ExcludeFromContext {
				continue
			}
			if msgs[j].Role == "assistant" {
				asst = msgs[j].Content
				break
			}
			if msgs[j].Role == "user" {
				break
			}
		}
		turns = append(turns, Turn{User: msgs[i].Content, Assistant: asst})
	}
	// 翻转为正序
	for l, r := 0, len(turns)-1; l < r; l, r = l+1, r-1 {
		turns[l], turns[r] = turns[r], turns[l]
	}
	return turns, nil
}

// ---- stats ----

// WSStat 统计维度:命中工作区 + 时间。
type WSStat struct {
	Date      string `json:"date"`
	Workspace string `json:"workspace"`
	Total     int    `json:"total"`
	OkCount   int    `json:"ok_count"`
}

// StatsByWSAndTime 按 (日期, 工作区) 聚合 assistant 消息。
func (s *Store) StatsByWSAndTime(ctx context.Context, from, to string) ([]WSStat, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DATE(created_at) AS d, COALESCE(workspace_path,'(none)') AS ws,
		        COUNT(*) AS total,
		        SUM(CASE WHEN status='ok' THEN 1 ELSE 0 END) AS okc
		 FROM messages
		 WHERE (? = '' OR created_at >= ?) AND (? = '' OR created_at < ?)
		 GROUP BY d, ws ORDER BY d, ws`,
		from, from, to, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WSStat
	for rows.Next() {
		var st WSStat
		var okc sql.NullInt64
		if err := rows.Scan(&st.Date, &st.Workspace, &st.Total, &okc); err != nil {
			return nil, err
		}
		st.OkCount = int(okc.Int64)
		out = append(out, st)
	}
	return out, rows.Err()
}
