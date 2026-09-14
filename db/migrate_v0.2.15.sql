-- v0.2.15: 会话引擎上下文
ALTER TABLE conversations
  ADD COLUMN engine_session_id VARCHAR(128) NULL,
  ADD COLUMN engine_used_tokens BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN engine_window_tokens BIGINT NOT NULL DEFAULT 0;
