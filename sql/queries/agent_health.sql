-- name: UpsertAgentHealth :one
-- Persist the latest health snapshot for an agent. One row per agent: a newer
-- report replaces the previous snapshot atomically.
INSERT INTO agent_health (
    agent_id, reported_at, agent_version, hostname, environment, status,
    cpu_user_percent, cpu_system_percent, cpu_idle_percent,
    memory_used_percent, disk_used_percent, snapshot
) VALUES (
    sqlc.arg('agent_id'), sqlc.arg('reported_at'), sqlc.arg('agent_version'),
    sqlc.arg('hostname'), sqlc.arg('environment'), sqlc.arg('status'),
    sqlc.arg('cpu_user_percent'), sqlc.arg('cpu_system_percent'),
    sqlc.arg('cpu_idle_percent'),
    sqlc.arg('memory_used_percent'), sqlc.arg('disk_used_percent'),
    sqlc.arg('snapshot')
)
ON CONFLICT (agent_id) DO UPDATE SET
    reported_at = EXCLUDED.reported_at,
    agent_version = EXCLUDED.agent_version,
    hostname = EXCLUDED.hostname,
    environment = EXCLUDED.environment,
    status = EXCLUDED.status,
    cpu_user_percent = EXCLUDED.cpu_user_percent,
    cpu_system_percent = EXCLUDED.cpu_system_percent,
    cpu_idle_percent = EXCLUDED.cpu_idle_percent,
    memory_used_percent = EXCLUDED.memory_used_percent,
    disk_used_percent = EXCLUDED.disk_used_percent,
    snapshot = EXCLUDED.snapshot,
    updated_at = now()
RETURNING agent_id, reported_at;

-- name: GetAgentHealthByAgentID :one
-- Fetch the latest health snapshot for one agent.
SELECT agent_id, reported_at, agent_version, hostname, environment, status,
       cpu_user_percent, cpu_system_percent, cpu_idle_percent,
       memory_used_percent, disk_used_percent, snapshot
FROM agent_health
WHERE agent_id = sqlc.arg('agent_id');

-- name: ListAgentHealth :many
-- Latest health snapshot for every agent with server context. Only safe
-- operational columns are projected.
SELECT ah.agent_id, ah.reported_at, ah.agent_version, ah.hostname, ah.environment,
       ah.status, ah.cpu_user_percent, ah.cpu_system_percent, ah.cpu_idle_percent,
       ah.memory_used_percent, ah.disk_used_percent, ah.snapshot,
       a.server_id, a.status AS agent_status, a.last_heartbeat, a.version AS agent_version_registered,
       s.name AS server_name, s.hostname AS server_hostname
FROM agent_health ah
JOIN agents a ON a.id = ah.agent_id
JOIN servers s ON s.id = a.server_id
ORDER BY ah.reported_at DESC;

-- name: ListAgentsWithHealth :many
-- Every active agent with its latest health snapshot, including agents that
-- have never reported (LEFT JOIN). Used to derive unhealthy-agent projections.
SELECT a.id, a.server_id, a.status AS agent_status, a.last_heartbeat, a.version,
       ah.reported_at, ah.status AS health_status, ah.disk_used_percent,
       s.hostname, s.environment
FROM agents a
LEFT JOIN agent_health ah ON ah.agent_id = a.id
JOIN servers s ON s.id = a.server_id
ORDER BY s.hostname ASC, a.created_at ASC;
