-- v0.2.9: 节点执行实时事件
USE colleague_avatar;

ALTER TABLE node_executions
  ADD COLUMN events_json JSON NULL AFTER output_json;
