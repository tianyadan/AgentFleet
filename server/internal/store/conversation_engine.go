package store

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// ConversationEngineMeta 引擎会话与最近一次真实上下文占用。
type ConversationEngineMeta struct {
	SessionID           string `json:"session_id"`
	UsedTokens          int64  `json:"used_tokens"`
	WindowTokens        int64  `json:"window_tokens"`
	TotalTokens         int64  `json:"total_tokens"`
	ContextSource       string `json:"context_source"`
	ContextUpdatedAt    int64  `json:"context_updated_at"`
	NeedsSystemReinject bool   `json:"needs_system_reinject"`
}

// GetConversationEngineMeta 读取会话引擎元数据。
func (s *Store) GetConversationEngineMeta(ctx context.Context, convID int64) (ConversationEngineMeta, error) {
	var m ConversationEngineMeta
	if convID <= 0 {
		return m, nil
	}
	var sid sql.NullString
	var reinject int
	err := s.db.QueryRowContext(ctx,
		`SELECT IFNULL(engine_session_id,''), IFNULL(engine_used_tokens,0), IFNULL(engine_window_tokens,0),
		        IFNULL(engine_total_tokens,0), IFNULL(engine_context_source,''),
		        IFNULL(UNIX_TIMESTAMP(engine_context_updated_at),0), IFNULL(needs_system_reinject,0)
		 FROM conversations WHERE id=?`, convID).
		Scan(&sid, &m.UsedTokens, &m.WindowTokens, &m.TotalTokens, &m.ContextSource, &m.ContextUpdatedAt, &reinject)
	if err == sql.ErrNoRows {
		return m, nil
	}
	if err != nil {
		// 兼容尚未迁移列的旧库
		err2 := s.db.QueryRowContext(ctx,
			`SELECT IFNULL(engine_session_id,''), IFNULL(engine_used_tokens,0), IFNULL(engine_window_tokens,0)
			 FROM conversations WHERE id=?`, convID).
			Scan(&sid, &m.UsedTokens, &m.WindowTokens)
		if err2 == sql.ErrNoRows {
			return m, nil
		}
		if err2 != nil {
			return m, err
		}
		if sid.Valid {
			m.SessionID = sid.String
		}
		return m, nil
	}
	if sid.Valid {
		m.SessionID = sid.String
	}
	m.NeedsSystemReinject = reinject != 0
	return m, nil
}

// UpdateConversationContextSnapshot 保存实际上下文快照；累计 session 用量单独保存。
func (s *Store) UpdateConversationContextSnapshot(ctx context.Context, convID int64, used, window, total int64, source string, observedAt time.Time) error {
	if convID <= 0 || strings.TrimSpace(source) == "" {
		return nil
	}
	if observedAt.IsZero() {
		observedAt = time.Now()
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE conversations SET engine_used_tokens=?, engine_window_tokens=?, engine_total_tokens=?,
		       engine_context_source=?, engine_context_updated_at=? WHERE id=?`,
		used, window, total, source, observedAt, convID)
	return err
}

// SetConversationNeedsSystemReinject 标记/清除压缩后需重带系统提示。
func (s *Store) SetConversationNeedsSystemReinject(ctx context.Context, convID int64, need bool) error {
	if convID <= 0 {
		return nil
	}
	v := 0
	if need {
		v = 1
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE conversations SET needs_system_reinject=? WHERE id=?`, v, convID)
	return err
}

// UpdateConversationEngineMeta 更新引擎 session / 上下文占用。
// session 只在当前为空时写入，避免子会话 id 覆盖主会话。
func (s *Store) UpdateConversationEngineMeta(ctx context.Context, convID int64, sessionID string, used, window int64) error {
	if convID <= 0 {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE conversations SET
		  engine_session_id = CASE
		    WHEN (engine_session_id IS NULL OR TRIM(IFNULL(engine_session_id,'')) = '') AND ? <> '' THEN ?
		    ELSE engine_session_id
		  END,
		  engine_used_tokens = CASE WHEN ? > 0 THEN ? ELSE engine_used_tokens END,
		  engine_window_tokens = CASE WHEN ? > 0 THEN ? ELSE engine_window_tokens END
		WHERE id=?`,
		sessionID, sessionID, used, used, window, window, convID)
	return err
}

// ClearConversationEngineMeta 新对话时清空引擎会话绑定。
func (s *Store) ClearConversationEngineMeta(ctx context.Context, convID int64) error {
	if convID <= 0 {
		return nil
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE conversations SET engine_session_id=NULL, engine_used_tokens=0, engine_window_tokens=0,
		 engine_total_tokens=0, engine_context_source='', engine_context_updated_at=NULL,
		 needs_system_reinject=0 WHERE id=?`, convID)
	// 兼容无 needs_system_reinject 列
	if err != nil {
		_, err = s.db.ExecContext(ctx,
			`UPDATE conversations SET engine_session_id=NULL, engine_used_tokens=0, engine_window_tokens=0 WHERE id=?`, convID)
	}
	return err
}

// ResetConversationUsedTokens compact / session reset：清零占用估算，保留 session 与窗口。
func (s *Store) ResetConversationUsedTokens(ctx context.Context, convID int64) error {
	if convID <= 0 {
		return nil
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE conversations SET engine_used_tokens=0, engine_context_source='', engine_context_updated_at=NULL WHERE id=?`, convID)
	return err
}
