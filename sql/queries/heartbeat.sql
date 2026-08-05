-- name: GetAgentByID :one
-- Fetch an agent by id, including the stored secret hash for verification.
SELECT id, server_id, secret, version, status, last_heartbeat, created_at, updated_at
FROM agents
WHERE id = sqlc.arg('id');

-- name: UpdateAgentLastHeartbeat :exec
-- Record the agent's latest heartbeat.
UPDATE agents
SET last_heartbeat = now(),
    updated_at = now()
WHERE id = sqlc.arg('id');
