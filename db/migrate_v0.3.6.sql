-- v0.3.6 前台 Ask 审计：一轮请求一条 visitor_ask_logs，关联 messages
USE colleague_avatar;

ALTER TABLE visitor_ask_logs
  ADD COLUMN user_message_id BIGINT NOT NULL DEFAULT 0 COMMENT '本次 Ask 对应的用户消息 ID' AFTER user_agent,
  ADD COLUMN assistant_message_id BIGINT NOT NULL DEFAULT 0 COMMENT '本次 Ask 对应的 AI 回复消息 ID' AFTER user_message_id,
  ADD COLUMN input_tokens BIGINT NOT NULL DEFAULT 0 COMMENT '本轮输入 Token' AFTER assistant_message_id,
  ADD COLUMN output_tokens BIGINT NOT NULL DEFAULT 0 COMMENT '本轮输出 Token' AFTER input_tokens,
  ADD COLUMN cached_tokens BIGINT NOT NULL DEFAULT 0 COMMENT '本轮缓存 Token' AFTER output_tokens,
  ADD COLUMN total_tokens BIGINT NOT NULL DEFAULT 0 COMMENT '本轮总 Token' AFTER cached_tokens,
  ADD COLUMN duration_ms INT NOT NULL DEFAULT 0 COMMENT '本轮完整耗时' AFTER total_tokens,
  ADD COLUMN status VARCHAR(20) NOT NULL DEFAULT 'processing' COMMENT 'processing/success/error/cancelled' AFTER duration_ms,
  ADD COLUMN error_message TEXT NULL COMMENT '失败原因' AFTER status,
  ADD COLUMN finished_at DATETIME NULL COMMENT '本次请求结束时间' AFTER error_message;

ALTER TABLE visitor_ask_logs
  ADD KEY idx_ask_user_message (user_message_id),
  ADD KEY idx_ask_assistant_message (assistant_message_id),
  ADD KEY idx_ask_conversation_time (conversation_id, created_at),
  ADD KEY idx_ask_visitor_time (visitor_id, created_at),
  ADD KEY idx_ask_status_time (status, created_at);
