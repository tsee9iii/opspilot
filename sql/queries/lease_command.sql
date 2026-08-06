-- name: LeaseNextCommand :one
-- Atomically lease the oldest pending, operator-approved command for an agent.
-- FOR UPDATE SKIP LOCKED guarantees only one leaser claims each row.
-- Commands awaiting confirmation (confirmation_status != 'approved') are
-- never leased.
UPDATE commands
SET status = 'leased',
    leased_at = now(),
    lease_owner = sqlc.arg('lease_owner')
WHERE id = (
    SELECT id
    FROM commands AS c
    WHERE c.agent_id = sqlc.arg('agent_id')
      AND c.status = 'pending'
      AND c.confirmation_status = 'approved'
    ORDER BY c.created_at
    LIMIT 1
    FOR UPDATE SKIP LOCKED
)
RETURNING id, agent_id, tool_name, payload, status, leased_at, lease_owner;

-- name: ExpireStaleLeases :exec
-- Return leases held by the agent that have outlived the lease TTL back to
-- pending so they can be leased again. Lazy expiry: run at lease time, never
-- from a background scheduler.
UPDATE commands
SET status = 'pending',
    lease_owner = NULL,
    leased_at = NULL
WHERE status = 'leased'
  AND lease_owner = sqlc.arg('lease_owner')
  AND leased_at < sqlc.arg('before');
