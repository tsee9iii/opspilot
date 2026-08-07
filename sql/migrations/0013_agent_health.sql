-- 0013_agent_health.sql
-- Latest per-agent health snapshot reported through POST /api/v1/agents/health.
--
-- One row per agent (agent_id is the primary key). History is deliberately not
-- retained: alert evaluation only needs the latest snapshot, and keeping the
-- table small makes the platform reliable and cheap. Normalized columns
-- (percentages, status) power alert evaluation without decoding JSON; the full
-- snapshot JSONB is retained for inventory tool projections.
--
-- The snapshot contains only safe operational data: no secrets, no file
-- contents, no process environments, no log tails and no HTTP response bodies.

CREATE TABLE agent_health (
    agent_id UUID PRIMARY KEY REFERENCES agents(id) ON DELETE CASCADE,
    reported_at TIMESTAMPTZ NOT NULL,
    agent_version TEXT NOT NULL DEFAULT '',
    hostname TEXT NOT NULL DEFAULT '',
    environment TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'unknown',
    cpu_user_percent DOUBLE PRECISION NOT NULL DEFAULT 0,
    cpu_system_percent DOUBLE PRECISION NOT NULL DEFAULT 0,
    cpu_idle_percent DOUBLE PRECISION NOT NULL DEFAULT 0,
    memory_used_percent DOUBLE PRECISION NOT NULL DEFAULT 0,
    disk_used_percent DOUBLE PRECISION NOT NULL DEFAULT 0,
    snapshot JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_agent_health_reported_at ON agent_health (reported_at DESC);
