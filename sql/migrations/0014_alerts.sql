-- 0014_alerts.sql
-- Central-side alerting derived from agent health snapshots and heartbeats.
--
-- Idempotency is enforced with a partial unique index: at most one OPEN alert
-- per (agent_id, rule_type). Repeated unhealthy reports update that open alert
-- (first_seen_at preserved, last_seen_at advanced); a recovered condition
-- resolves it, and the next unhealthy report opens a fresh one. This keeps
-- evaluation idempotent without locking or application-level coordination.
--
-- Acknowledge is an operator API action; the MCP process has no path to
-- acknowledge or resolve alerts.

CREATE TABLE alerts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    server_id UUID REFERENCES servers(id) ON DELETE CASCADE,
    rule_type TEXT NOT NULL,
    severity TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'open',
    message TEXT NOT NULL DEFAULT '',
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at TIMESTAMPTZ,
    acknowledged_at TIMESTAMPTZ,
    acknowledged_by TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_alerts_open_agent_rule
    ON alerts (agent_id, rule_type) WHERE status = 'open';

CREATE INDEX idx_alerts_status_seen ON alerts (status, last_seen_at DESC);
