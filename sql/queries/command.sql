-- name: CreateCommand :one
-- Persist a new command in pending state.
INSERT INTO commands (agent_id, tool_name, payload, status)
VALUES (
    sqlc.arg('agent_id'),
    sqlc.arg('tool_name'),
    sqlc.arg('payload'),
    'pending'
)
RETURNING id, status;
