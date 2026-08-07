-- name: UpsertOpenAlert :one
-- Open an alert for an (agent_id, rule_type) pair, or advance the existing
-- open alert if one is already present. created is true only when this call
-- inserted a brand-new alert; a repeated unhealthy report updates last_seen_at
-- and leaves first_seen_at untouched, so evaluation never duplicates alerts.
INSERT INTO alerts (agent_id, server_id, rule_type, severity, status, message)
VALUES (
    sqlc.arg('agent_id'), sqlc.arg('server_id'), sqlc.arg('rule_type'),
    sqlc.arg('severity'), 'open', sqlc.arg('message')
)
ON CONFLICT (agent_id, rule_type) WHERE status = 'open'
DO UPDATE SET last_seen_at = now(), message = EXCLUDED.message, updated_at = now()
RETURNING id, agent_id, server_id, rule_type, severity, status, message,
          first_seen_at, last_seen_at,
          (first_seen_at = last_seen_at) AS created;

-- name: ResolveOpenAlert :one
-- Resolve the open/acknowledged alert for an (agent_id, rule_type) pair.
-- Returns the resolved row so the evaluator can emit a resolved event; a
-- missing or already-resolved alert returns no row.
UPDATE alerts
SET status = 'resolved', resolved_at = now(), updated_at = now()
WHERE agent_id = sqlc.arg('agent_id')
  AND rule_type = sqlc.arg('rule_type')
  AND status IN ('open', 'acknowledged')
RETURNING id, agent_id, server_id, rule_type, severity, status, message,
          first_seen_at, last_seen_at, resolved_at, acknowledged_at, acknowledged_by;

-- name: ListAlerts :many
-- Alert projection with optional status, severity, agent and server filters,
-- always capped by limit. Ordered newest first.
SELECT id, agent_id, server_id, rule_type, severity, status, message,
       first_seen_at, last_seen_at, resolved_at, acknowledged_at, acknowledged_by
FROM alerts
WHERE (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('severity')::text IS NULL OR severity = sqlc.narg('severity')::text)
  AND (sqlc.narg('agent_id')::uuid IS NULL OR agent_id = sqlc.narg('agent_id')::uuid)
  AND (sqlc.narg('server_id')::uuid IS NULL OR server_id = sqlc.narg('server_id')::uuid)
ORDER BY last_seen_at DESC
LIMIT sqlc.arg('limit');

-- name: GetAlertByID :one
-- Fetch a single alert by id.
SELECT id, agent_id, server_id, rule_type, severity, status, message,
       first_seen_at, last_seen_at, resolved_at, acknowledged_at, acknowledged_by
FROM alerts
WHERE id = sqlc.arg('id');

-- name: AcknowledgeAlert :one
-- Acknowledge an open alert. Only open alerts transition; acknowledging an
-- already-acknowledged alert is a no-op handled by the caller. Acknowledged
-- alerts remain visible and unresolved until a recovery resolves them.
UPDATE alerts
SET status = 'acknowledged', acknowledged_at = now(),
    acknowledged_by = sqlc.arg('acknowledged_by'), updated_at = now()
WHERE id = sqlc.arg('id') AND status = 'open'
RETURNING id, agent_id, server_id, rule_type, severity, status, message,
          first_seen_at, last_seen_at, resolved_at, acknowledged_at, acknowledged_by;

-- name: ListAgentsForEvaluation :many
-- Active (online/offline) agents with the signal the evaluator needs:
-- heartbeat freshness, latest health report time, health status, disk usage and
-- the full snapshot (for project health). Unregistered agents are never
-- evaluated.
SELECT a.id, a.server_id, a.status, a.last_heartbeat,
       ah.reported_at AS last_health_at, ah.status AS health_status,
       ah.disk_used_percent, ah.snapshot
FROM agents a
LEFT JOIN agent_health ah ON ah.agent_id = a.id
WHERE a.status IN ('online', 'offline');
