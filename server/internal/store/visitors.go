package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"strings"
	"time"
)

// Visitor 前台来访者（Cookie 凭证绑定）。
type Visitor struct {
	ID                    int64     `json:"id"`
	PublicID              string    `json:"public_id"`
	Name                  string    `json:"name"`
	CurrentConversationID int64     `json:"current_conversation_id"`
	CreatedAt             time.Time `json:"created_at"`
	LastSeenAt            time.Time `json:"last_seen_at"`
}

// ensureReceptionistSchema 前台助理标记、访客表、会话 visitor_id。
func (s *Store) ensureReceptionistSchema(ctx context.Context) error {
	for _, col := range []struct{ table, name, ddl string }{
		{"managed_agents", "is_receptionist", `ALTER TABLE managed_agents ADD COLUMN is_receptionist TINYINT NOT NULL DEFAULT 0 AFTER status`},
		{"conversations", "visitor_id", `ALTER TABLE conversations ADD COLUMN visitor_id BIGINT NULL AFTER agent_id`},
	} {
		var n int
		if err := s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM information_schema.COLUMNS
			 WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?`,
			col.table, col.name,
		).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			if _, err := s.db.ExecContext(ctx, col.ddl); err != nil {
				return err
			}
		}
	}
	if _, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS visitors (
		  id                      BIGINT       NOT NULL AUTO_INCREMENT,
		  public_id               VARCHAR(64)  NOT NULL,
		  name                    VARCHAR(64)  NOT NULL,
		  current_conversation_id BIGINT       NULL,
		  created_at              DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
		  last_seen_at            DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
		  PRIMARY KEY (id),
		  UNIQUE KEY uk_visitor_public (public_id),
		  KEY idx_visitor_seen (last_seen_at)
		) ENGINE=InnoDB`); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS visitor_ask_logs (
		  id               BIGINT        NOT NULL AUTO_INCREMENT,
		  visitor_id       BIGINT        NOT NULL,
		  conversation_id  BIGINT        NOT NULL DEFAULT 0,
		  agent_id         BIGINT        NOT NULL DEFAULT 0,
		  ip               VARCHAR(64)   NOT NULL DEFAULT '',
		  user_agent       VARCHAR(512)  NOT NULL DEFAULT '',
		  created_at       DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
		  PRIMARY KEY (id),
		  KEY idx_val_visitor (visitor_id, created_at),
		  KEY idx_val_ip (ip, created_at)
		) ENGINE=InnoDB`); err != nil {
		return err
	}
	return nil
}

// NewVisitorPublicID 生成访客公开 id。
func NewVisitorPublicID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// CreateVisitor 登记来访者。
func (s *Store) CreateVisitor(ctx context.Context, name string) (*Visitor, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, sql.ErrNoRows
	}
	if len([]rune(name)) > 32 {
		name = string([]rune(name)[:32])
	}
	pid, err := NewVisitorPublicID()
	if err != nil {
		return nil, err
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO visitors (public_id, name) VALUES (?,?)`, pid, name)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetVisitorByID(ctx, id)
}

// GetVisitorByID 按主键取访客。
func (s *Store) GetVisitorByID(ctx context.Context, id int64) (*Visitor, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, public_id, name, IFNULL(current_conversation_id,0), created_at, last_seen_at
		 FROM visitors WHERE id=?`, id)
	return scanVisitor(row)
}

// GetVisitorByPublicID 按 Cookie 中的 public_id 取访客。
func (s *Store) GetVisitorByPublicID(ctx context.Context, publicID string) (*Visitor, error) {
	publicID = strings.TrimSpace(publicID)
	if publicID == "" {
		return nil, nil
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT id, public_id, name, IFNULL(current_conversation_id,0), created_at, last_seen_at
		 FROM visitors WHERE public_id=?`, publicID)
	v, err := scanVisitor(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return v, err
}

func scanVisitor(row interface{ Scan(dest ...any) error }) (*Visitor, error) {
	var v Visitor
	err := row.Scan(&v.ID, &v.PublicID, &v.Name, &v.CurrentConversationID, &v.CreatedAt, &v.LastSeenAt)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// UpdateVisitorName 更新姓名并刷新 last_seen。
func (s *Store) UpdateVisitorName(ctx context.Context, id int64, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return sql.ErrNoRows
	}
	if len([]rune(name)) > 32 {
		name = string([]rune(name)[:32])
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE visitors SET name=?, last_seen_at=NOW() WHERE id=?`, name, id)
	return err
}

// TouchVisitor 刷新最近活跃时间。
func (s *Store) TouchVisitor(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE visitors SET last_seen_at=NOW() WHERE id=?`, id)
	return err
}

// SetVisitorCurrentConversation 绑定当前前台会话。
func (s *Store) SetVisitorCurrentConversation(ctx context.Context, visitorID, convID int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE visitors SET current_conversation_id=?, last_seen_at=NOW() WHERE id=?`, convID, visitorID)
	return err
}

// CreateVisitorConversation 为访客新建与前台助理的隔离会话。
func (s *Store) CreateVisitorConversation(ctx context.Context, visitorID, agentID int64, userIP string) (int64, error) {
	if userIP == "" {
		userIP = "visitor"
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO conversations (user_ip, mode, agent_id, visitor_id) VALUES (?,?,?,?)`,
		userIP, "chat", agentID, visitorID)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ConversationHasVisitor 判断会话是否为前台访客会话（禁止人工授权）。
func (s *Store) ConversationHasVisitor(ctx context.Context, convID int64) (bool, error) {
	if convID <= 0 {
		return false, nil
	}
	var vid sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		`SELECT visitor_id FROM conversations WHERE id=?`, convID).Scan(&vid)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		// 列未迁移时视为非访客
		return false, nil
	}
	return vid.Valid && vid.Int64 > 0, nil
}

// InsertVisitorAskLog 记录一次前台提问的 IP/设备。
func (s *Store) InsertVisitorAskLog(ctx context.Context, visitorID, convID, agentID int64, ip, ua string) error {
	if len(ua) > 512 {
		ua = ua[:512]
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO visitor_ask_logs (visitor_id, conversation_id, agent_id, ip, user_agent) VALUES (?,?,?,?,?)`,
		visitorID, convID, agentID, ip, ua)
	return err
}

// GetReceptionist 返回当前前台助理（最多一个）。
func (s *Store) GetReceptionist(ctx context.Context) (*ManagedAgent, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+managedSelectCols+` FROM managed_agents WHERE IFNULL(is_receptionist,0)=1 ORDER BY id ASC LIMIT 1`)
	a, err := scanManagedAgent(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// SetReceptionist 设为唯一前台助理；agentID=0 表示清除。
func (s *Store) SetReceptionist(ctx context.Context, agentID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `UPDATE managed_agents SET is_receptionist=0 WHERE IFNULL(is_receptionist,0)=1`); err != nil {
		return err
	}
	if agentID > 0 {
		res, err := tx.ExecContext(ctx, `UPDATE managed_agents SET is_receptionist=1 WHERE id=?`, agentID)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return sql.ErrNoRows
		}
	}
	return tx.Commit()
}
