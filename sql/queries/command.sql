-- name: CreateCommand :one
-- Persist a new command in pending state. confirmation_status is resolved
-- from the target tool's capability ('approved' or 'pending'). source and
-- requested_by are the immutable audit origin of the command.
INSERT INTO commands (agent_id, tool_name, payload, status, confirmation_status, source, requested_by)
VALUES (
    sqlc.arg('agent_id'),
    sqlc.arg('tool_name'),
    sqlc.arg('payload'),
    'pending',
    sqlc.arg('confirmation_status'),
    sqlc.arg('source'),
    sqlc.arg('requested_by')
)
RETURNING id, status;

-- name: GetCommandResult :one
-- Fetch a command's full current state and result. The result payload is
-- returned exactly as stored (opaque JSON); it is never transformed.
SELECT id, agent_id, tool_name, payload, status, result, error,
       confirmation_status, leased_at, lease_owner, started_at, completed_at,
       confirmed_at, created_at, updated_at,
       source, requested_by, requested_at, approved_by, approved_at, approval_note
FROM commands
WHERE id = sqlc.arg('id');
