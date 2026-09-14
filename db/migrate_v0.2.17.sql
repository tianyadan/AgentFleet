-- v0.2.17: 清除曾被任务拆分污染的 Claude engine_session，避免 Ask 只回 JSON 计划数组就中断。
-- 真实 Ask 完成后会重新写入 session；Codex 不受影响。
UPDATE conversations c
INNER JOIN managed_agents a ON a.conversation_id = c.id
SET c.engine_session_id = NULL
WHERE LOWER(IFNULL(a.engine, '')) = 'claude'
  AND c.engine_session_id IS NOT NULL
  AND TRIM(c.engine_session_id) <> '';
