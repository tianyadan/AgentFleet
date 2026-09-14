package store

import (
	"context"
	"database/sql"
)

// ConversationsPage 分页查询会话(按 updated_at 倒序)。page 从 1 起;pageSize 默认 10。
func (s *Store) ConversationsPage(ctx context.Context, page, pageSize int, userIP string) ([]ConvWithPreview, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	if pageSize > 50 {
		pageSize = 50
	}
	where := "WHERE 1=1"
	args := []interface{}{}
	if userIP != "" {
		where += " AND c.user_ip = ?"
		args = append(args, userIP)
	}
	var total int
	countQ := `SELECT COUNT(*) FROM conversations c ` + where
	if err := s.db.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	q := `SELECT c.id, c.user_ip, c.mode, c.created_at, c.updated_at,
	        (SELECT mc.content FROM messages mc WHERE mc.conversation_id = c.id AND mc.role='user' ORDER BY mc.id DESC LIMIT 1) AS preview
	 FROM conversations c ` + where + `
	 ORDER BY c.updated_at DESC, c.id DESC
	 LIMIT ? OFFSET ?`
	args2 := append(append([]interface{}{}, args...), pageSize, offset)
	rows, err := s.db.QueryContext(ctx, q, args2...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []ConvWithPreview
	for rows.Next() {
		var c ConvWithPreview
		var preview sql.NullString
		if err := rows.Scan(&c.ID, &c.UserIP, &c.Mode, &c.CreatedAt, &c.UpdatedAt, &preview); err != nil {
			return nil, 0, err
		}
		if preview.Valid {
			c.Preview = preview.String
		}
		out = append(out, c)
	}
	return out, total, rows.Err()
}

// DayStat 按日聚合。
type DayStat struct {
	Date      string `json:"date"`
	Total     int    `json:"total"`
	OkCount   int    `json:"ok_count"`
	FailCount int    `json:"fail_count"`
}

// IPDayStat 某日按 Client IP 聚合。
type IPDayStat struct {
	UserIP    string `json:"user_ip"`
	Total     int    `json:"total"`
	OkCount   int    `json:"ok_count"`
	FailCount int    `json:"fail_count"`
}

// StatsByDay 按日期聚合 assistant 消息,最新日在前。
func (s *Store) StatsByDay(ctx context.Context) ([]DayStat, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT DATE(m.created_at) AS d,
		       COUNT(*) AS total,
		       SUM(CASE WHEN m.status='ok' THEN 1 ELSE 0 END) AS okc,
		       SUM(CASE WHEN m.status<>'ok' THEN 1 ELSE 0 END) AS failc
		FROM messages m
		WHERE m.role='assistant'
		GROUP BY d
		ORDER BY d DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DayStat
	for rows.Next() {
		var st DayStat
		var okc, failc sql.NullInt64
		if err := rows.Scan(&st.Date, &st.Total, &okc, &failc); err != nil {
			return nil, err
		}
		st.OkCount = int(okc.Int64)
		st.FailCount = int(failc.Int64)
		out = append(out, st)
	}
	return out, rows.Err()
}

// StatsByDayIP 某日各 IP 的成功/失败次数。
func (s *Store) StatsByDayIP(ctx context.Context, date string) ([]IPDayStat, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.user_ip,
		       COUNT(*) AS total,
		       SUM(CASE WHEN m.status='ok' THEN 1 ELSE 0 END) AS okc,
		       SUM(CASE WHEN m.status<>'ok' THEN 1 ELSE 0 END) AS failc
		FROM messages m
		JOIN conversations c ON c.id = m.conversation_id
		WHERE m.role='assistant' AND DATE(m.created_at) = ?
		GROUP BY c.user_ip
		ORDER BY total DESC, c.user_ip`, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []IPDayStat
	for rows.Next() {
		var st IPDayStat
		var okc, failc sql.NullInt64
		if err := rows.Scan(&st.UserIP, &st.Total, &okc, &failc); err != nil {
			return nil, err
		}
		st.OkCount = int(okc.Int64)
		st.FailCount = int(failc.Int64)
		out = append(out, st)
	}
	return out, rows.Err()
}
