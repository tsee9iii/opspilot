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
