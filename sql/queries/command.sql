-- name: CreateCommand :one
-- Persist a new command in pending state. confirmation_status is resolved
-- from the target tool's capability ('approved' or 'pending').
INSERT INTO commands (agent_id, tool_name, payload, status, confirmation_status)
VALUES (
    sqlc.arg('agent_id'),
    sqlc.arg('tool_name'),
    sqlc.arg('payload'),
    'pending',
    sqlc.arg('confirmation_status')
)
RETURNING id, status;

-- name: GetCommandResult :one
-- Fetch a command's full current state and result. The result payload is
-- returned exactly as stored (opaque JSON); it is never transformed.
SELECT id, agent_id, tool_name, payload, status, result, error,
       confirmation_status, leased_at, lease_owner, started_at, completed_at,
       confirmed_at, created_at, updated_at
FROM commands
WHERE id = sqlc.arg('id');
