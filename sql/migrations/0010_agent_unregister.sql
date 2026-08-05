-- 0010_agent_unregister.sql
-- Agent lifecycle: agents can be unregistered. Restricts status values to
-- online / offline / unregistered and migrates existing agents to online.

ALTER TABLE agents
    DROP CONSTRAINT IF EXISTS agents_status_check;

UPDATE agents
SET status = 'online'
WHERE status = 'offline';

ALTER TABLE agents
    ADD CONSTRAINT agents_status_check
    CHECK (status IN ('online', 'offline', 'unregistered'));
