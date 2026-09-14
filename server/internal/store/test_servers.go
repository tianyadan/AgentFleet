package store

import (
	"context"
	"database/sql"
	"time"
)

// TestServer 测试服务器配置(列表不返回密码明文)。
type TestServer struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Host        string    `json:"host"`
	Port        int       `json:"port"`
	Username    string    `json:"username"`
	PasswordSet bool      `json:"password_set"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type TestServerAllowedIP struct {
	ID        int64     `json:"id"`
	IP        string    `json:"ip"`
	Note      string    `json:"note"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Store) ListTestServers(ctx context.Context) ([]TestServer, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, host, port, username,
		       (password_enc IS NOT NULL AND password_enc <> '') AS pwd,
		       enabled, created_at, updated_at
		FROM test_servers ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TestServer
	for rows.Next() {
		var t TestServer
		var en int
		if err := rows.Scan(&t.ID, &t.Name, &t.Host, &t.Port, &t.Username, &t.PasswordSet, &en, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		t.Enabled = en == 1
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) GetTestServerSecret(ctx context.Context, id int64) (host string, port int, user, passEnc string, err error) {
	err = s.db.QueryRowContext(ctx,
		`SELECT host, port, username, password_enc FROM test_servers WHERE id=? AND enabled=1`, id,
	).Scan(&host, &port, &user, &passEnc)
	return
}

func (s *Store) InsertTestServer(ctx context.Context, name, host string, port int, username, passEnc string) (int64, error) {
	if port <= 0 {
		port = 22
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO test_servers (name, host, port, username, password_enc, enabled) VALUES (?,?,?,?,?,1)`,
		name, host, port, username, passEnc)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateTestServer(ctx context.Context, id int64, name, host string, port int, username, passEnc string, updatePass bool) error {
	if port <= 0 {
		port = 22
	}
	if updatePass {
		_, err := s.db.ExecContext(ctx,
			`UPDATE test_servers SET name=?, host=?, port=?, username=?, password_enc=? WHERE id=?`,
			name, host, port, username, passEnc, id)
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE test_servers SET name=?, host=?, port=?, username=? WHERE id=?`,
		name, host, port, username, id)
	return err
}

func (s *Store) DeleteTestServer(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM test_servers WHERE id=?`, id)
	return err
}

func (s *Store) ListTestServerAllowedIPs(ctx context.Context) ([]TestServerAllowedIP, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, ip, note, enabled, created_at FROM test_server_allowed_ips ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TestServerAllowedIP
	for rows.Next() {
		var t TestServerAllowedIP
		var en int
		if err := rows.Scan(&t.ID, &t.IP, &t.Note, &en, &t.CreatedAt); err != nil {
			return nil, err
		}
		t.Enabled = en == 1
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) UpsertTestServerAllowedIP(ctx context.Context, ip, note string) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO test_server_allowed_ips (ip, note, enabled) VALUES (?,?,1)
		ON DUPLICATE KEY UPDATE note=VALUES(note), enabled=1`, ip, note)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	if id == 0 {
		_ = s.db.QueryRowContext(ctx, `SELECT id FROM test_server_allowed_ips WHERE ip=?`, ip).Scan(&id)
	}
	return id, nil
}

func (s *Store) DeleteTestServerAllowedIP(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM test_server_allowed_ips WHERE id=?`, id)
	return err
}

func (s *Store) IsTestServerIPAllowed(ctx context.Context, ip string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM test_server_allowed_ips WHERE enabled=1 AND ip=?`, ip).Scan(&n)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return n > 0, err
}
