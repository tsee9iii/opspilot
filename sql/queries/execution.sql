-- name: GetCommandByID :one
SELECT id, agent_id, status
FROM commands
WHERE id = sqlc.arg('id');

-- name: StartCommand :one
-- Transition leased -> running.
UPDATE commands
SET status = 'running',
    started_at = now(),
    updated_at = now()
WHERE id = sqlc.arg('id')
  AND agent_id = sqlc.arg('agent_id')
  AND status = 'leased'
RETURNING id, status;

-- name: CompleteCommand :one
-- Transition running -> completed.
UPDATE commands
SET status = 'completed',
    completed_at = now(),
    result = sqlc.arg('result'),
    updated_at = now()
WHERE id = sqlc.arg('id')
  AND agent_id = sqlc.arg('agent_id')
  AND status = 'running'
RETURNING id, status;

-- name: FailCommand :one
-- Transition running -> failed.
UPDATE commands
SET status = 'failed',
    completed_at = now(),
    error = sqlc.arg('error'),
    updated_at = now()
WHERE id = sqlc.arg('id')
  AND agent_id = sqlc.arg('agent_id')
  AND status = 'running'
RETURNING id, status;
