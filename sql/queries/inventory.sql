-- name: ListServers :many
-- Read-only inventory projection: each server with its agent totals.
SELECT
    s.id,
    s.name,
    s.hostname,
    s.environment,
    s.status,
    COUNT(a.id)::bigint AS agent_count,
    COUNT(a.id) FILTER (WHERE a.status = 'online')::bigint AS online_agent_count
FROM servers AS s
LEFT JOIN agents AS a ON a.server_id = s.id
GROUP BY s.id
ORDER BY s.name ASC;

-- name: ListAgents :many
-- Read-only inventory projection: agents with their server context. Both
-- filters are optional; an empty server_id or status selects all agents.
SELECT
    a.id,
    a.server_id,
    a.version,
    a.status,
    a.last_heartbeat,
    s.name AS server_name,
    s.hostname,
    s.environment
FROM agents AS a
JOIN servers AS s ON s.id = a.server_id
WHERE (sqlc.narg('server_id')::uuid IS NULL OR a.server_id = sqlc.narg('server_id')::uuid)
  AND (sqlc.narg('status')::text IS NULL OR a.status = sqlc.narg('status')::text)
ORDER BY s.hostname ASC, a.created_at ASC;

-- name: ListCommands :many
-- Read-only inventory projection: lightweight command summaries. Filters are
-- optional; the result set is always capped by limit.
SELECT
    id,
    agent_id,
    tool_name,
    status,
    created_at
FROM commands
WHERE (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('agent_id')::uuid IS NULL OR agent_id = sqlc.narg('agent_id')::uuid)
ORDER BY created_at DESC
LIMIT sqlc.arg('limit');
