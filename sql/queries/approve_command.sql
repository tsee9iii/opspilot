-- name: ApproveCommand :one
-- Approve a command awaiting confirmation. Only pending-confirmation commands
-- match; a command that is already approved is a no-op handled by the caller.
-- The approval audit fields are written exactly once, at the pending -> approved
-- transition, so a duplicate approval never overwrites the original actor or
-- timestamp.
UPDATE commands
SET confirmation_status = 'approved',
    confirmed_at = now(),
    approved_at = now(),
    approved_by = sqlc.arg('approved_by'),
    approval_note = sqlc.arg('approval_note'),
    updated_at = now()
WHERE id = sqlc.arg('id')
  AND confirmation_status = 'pending'
RETURNING id, agent_id, confirmation_status, confirmed_at, approved_at, approved_by;
