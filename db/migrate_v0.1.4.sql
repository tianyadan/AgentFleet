-- v0.1.4: managed_agents.run_started_at for live run duration
USE colleague_avatar;

SET @col_exists := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = 'colleague_avatar'
    AND TABLE_NAME = 'managed_agents'
    AND COLUMN_NAME = 'run_started_at'
);
SET @sql := IF(@col_exists = 0,
  'ALTER TABLE managed_agents ADD COLUMN run_started_at DATETIME NULL',
  'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
