-- 0004_command_execution.sql
-- Command execution: track lifecycle timestamps.

ALTER TABLE commands
    ADD COLUMN started_at    TIMESTAMPTZ,
    ADD COLUMN completed_at  TIMESTAMPTZ;
