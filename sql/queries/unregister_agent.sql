-- name: MarkAgentUnregistered :one
-- Transition an agent to the unregistered lifecycle state. A no-op when the
-- agent is already unregistered; returns no rows when the agent does not exist.
UPDATE agents
SET status = 'unregistered',
    updated_at = now()
WHERE id = sqlc.arg('id')
RETURNING id, status;

-- name: DeleteAgentCapabilities :exec
-- Remove all capabilities advertised by an agent.
DELETE FROM capabilities
WHERE agent_id = sqlc.arg('agent_id');
