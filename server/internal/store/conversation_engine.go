package store

import (
	"context"
	"database/sql"
)

// ConversationEngineMeta 引擎会话与最近一次真实上下文占用。
type ConversationEngineMeta struct {
	SessionID           string `json:"session_id"`
	UsedTokens          int64  `json:"used_tokens"`
	WindowTokens        int64  `json:"window_tokens"`
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
		        IFNULL(needs_system_reinject,0)
		 FROM conversations WHERE id=?`, convID).
		Scan(&sid, &m.UsedTokens, &m.WindowTokens, &reinject)
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
func (s *Store) UpdateConversationEngineMeta(ctx context.Context, convID int64, sessionID string, used, window int64) error {
	if convID <= 0 {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE conversations SET
		  engine_session_id = CASE WHEN ? <> '' THEN ? ELSE engine_session_id END,
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
		`UPDATE conversations SET engine_session_id=NULL, engine_used_tokens=0, engine_window_tokens=0, needs_system_reinject=0 WHERE id=?`, convID)
	// 兼容无 needs_system_reinject 列
	if err != nil {
		_, err = s.db.ExecContext(ctx,
			`UPDATE conversations SET engine_session_id=NULL, engine_used_tokens=0, engine_window_tokens=0 WHERE id=?`, convID)
	}
	return err
}
