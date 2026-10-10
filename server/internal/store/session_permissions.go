package store

import (
	"context"
	"strings"

	"atolla/server/internal/permission"
)

func (s *Store) ensurePermissionGatewaySchema(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS session_permissions (
		  id                 BIGINT        NOT NULL AUTO_INCREMENT,
		  conversation_id    BIGINT        NOT NULL,
		  action_type        VARCHAR(64)   NOT NULL,
		  command_signature  VARCHAR(768)  NOT NULL,
		  working_dir        VARCHAR(1024) NOT NULL DEFAULT '',
		  environment        VARCHAR(128)  NOT NULL DEFAULT '',
		  resource_scope     VARCHAR(1024) NOT NULL DEFAULT '',
		  created_at         DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
		  PRIMARY KEY (id),
		  KEY idx_conv_sig (conversation_id, command_signature(191)),
		  CONSTRAINT fk_sp_conv FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE
		) ENGINE=InnoDB`); err != nil {
		return err
	}
	for _, col := range []struct{ name, ddl string }{
		{"engine", `ALTER TABLE command_audits ADD COLUMN engine VARCHAR(32) NOT NULL DEFAULT ''`},
		{"action_type", `ALTER TABLE command_audits ADD COLUMN action_type VARCHAR(64) NOT NULL DEFAULT ''`},
		{"environment", `ALTER TABLE command_audits ADD COLUMN environment VARCHAR(128) NOT NULL DEFAULT ''`},
		{"risk_score", `ALTER TABLE command_audits ADD COLUMN risk_score DOUBLE NOT NULL DEFAULT 0`},
	} {
		var n int
		if err := s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM information_schema.COLUMNS
			 WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'command_audits' AND COLUMN_NAME = ?`, col.name,
		).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			if _, err := s.db.ExecContext(ctx, col.ddl); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) Grant(convID int64, g permission.SessionGrant) error {
	if convID <= 0 {
		return nil
	}
	_, err := s.db.ExecContext(context.Background(),
		`INSERT INTO session_permissions
		 (conversation_id, action_type, command_signature, working_dir, environment, resource_scope)
		 VALUES (?,?,?,?,?,?)`,
		convID, g.ActionType, g.CommandSignature, g.WorkingDir, g.Environment, g.ResourceScope)
	return err
}

func (s *Store) Match(convID int64, sig string) (bool, error) {
	if convID <= 0 || strings.TrimSpace(sig) == "" {
		return false, nil
	}
	var n int
	err := s.db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM session_permissions WHERE conversation_id=? AND command_signature=?`,
		convID, sig).Scan(&n)
	return n > 0, err
}

func (s *Store) Clear(convID int64) error {
	if convID <= 0 {
		return nil
	}
	_, err := s.db.ExecContext(context.Background(),
		`DELETE FROM session_permissions WHERE conversation_id=?`, convID)
	return err
}
