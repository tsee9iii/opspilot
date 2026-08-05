-- name: ApproveCommand :one
-- Approve a command awaiting confirmation. Only pending-confirmation commands
-- match; a command that is already approved is a no-op handled by the caller.
UPDATE commands
SET confirmation_status = 'approved',
    confirmed_at = now(),
    updated_at = now()
WHERE id = sqlc.arg('id')
  AND confirmation_status = 'pending'
RETURNING id, confirmation_status, confirmed_at;
