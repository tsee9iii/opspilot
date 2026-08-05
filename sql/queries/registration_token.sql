-- name: CreateRegistrationToken :one
-- Insert a registration token. Only the HMAC of the token is ever stored.
INSERT INTO registration_tokens (token_hash, environment, expires_at)
VALUES (
    sqlc.arg('token_hash'),
    sqlc.arg('environment'),
    sqlc.arg('expires_at')
)
RETURNING id, token_hash, environment, expires_at, revoked_at, created_at;

-- name: GetRegistrationTokenByHash :one
-- Look up a registration token by its HMAC.
SELECT id, token_hash, environment, expires_at, revoked_at, created_at
FROM registration_tokens
WHERE token_hash = sqlc.arg('token_hash')
LIMIT 1;

-- name: DeleteRegistrationTokenByHash :one
-- Atomically consume a registration token. Returns 0 rows when the token
-- does not exist (already consumed or never issued).
DELETE FROM registration_tokens
WHERE token_hash = sqlc.arg('token_hash')
RETURNING id;

-- name: RevokeRegistrationToken :exec
-- Revoke an unconsumed registration token.
UPDATE registration_tokens
SET revoked_at = now()
WHERE id = sqlc.arg('id');

-- name: ListRegistrationTokens :many
-- List all registration tokens, most recently created first.
SELECT id, token_hash, environment, expires_at, revoked_at, created_at
FROM registration_tokens
ORDER BY created_at DESC, id;
