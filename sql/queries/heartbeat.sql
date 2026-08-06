-- name: GetAgentByID :one
-- Fetch an agent by id, including its stored secret hash and signing key.
SELECT id, server_id, secret, signing_key, version, status, last_heartbeat, created_at, updated_at
FROM agents
WHERE id = sqlc.arg('id');

-- name: UpdateAgentLastHeartbeat :exec
-- Record the agent's latest heartbeat and mark an offline agent back online.
-- Only active agents (offline/online) are touched: an unregistered agent or any
-- future terminal status is never resurrected by a heartbeat.
UPDATE agents
SET last_heartbeat = now(),
    updated_at = now(),
    status = CASE WHEN status = 'offline' THEN 'online' ELSE status END
WHERE id = sqlc.arg('id')
  AND status IN ('offline', 'online');
