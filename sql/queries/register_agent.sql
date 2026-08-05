-- name: UpsertServer :one
-- Find a server by hostname and environment, or create it.
-- TODO: Server.name currently defaults to hostname. A custom display name
-- will be supported in a later version.
INSERT INTO servers (name, hostname, environment, status)
VALUES (
    sqlc.arg('hostname'),
    sqlc.arg('hostname'),
    sqlc.arg('environment'),
    'unknown'
)
ON CONFLICT (hostname, environment)
DO UPDATE SET updated_at = now()
RETURNING id;

-- name: CreateAgent :one
-- Insert a new agent linked to an existing server.
INSERT INTO agents (id, server_id, secret, version)
VALUES (
    sqlc.arg('id'),
    sqlc.arg('server_id'),
    sqlc.arg('secret'),
    sqlc.arg('version')
)
RETURNING id, status;
