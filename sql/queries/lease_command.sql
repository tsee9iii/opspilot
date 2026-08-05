-- name: LeaseNextCommand :one
-- Atomically lease the oldest pending command for an agent.
-- FOR UPDATE SKIP LOCKED guarantees only one leaser claims each row.
UPDATE commands
SET status = 'leased',
    leased_at = now(),
    lease_owner = sqlc.arg('lease_owner')
WHERE id = (
    SELECT id
    FROM commands AS c
    WHERE c.agent_id = sqlc.arg('agent_id')
      AND c.status = 'pending'
    ORDER BY c.created_at
    LIMIT 1
    FOR UPDATE SKIP LOCKED
)
RETURNING id, agent_id, tool_name, payload, status, leased_at, lease_owner;
