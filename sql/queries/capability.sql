-- name: UpsertCapability :exec
-- Register or refresh one tool capability for an agent.
INSERT INTO capabilities (agent_id, tool_name, version, description)
VALUES (
    sqlc.arg('agent_id'),
    sqlc.arg('tool_name'),
    sqlc.arg('version'),
    sqlc.arg('description')
)
ON CONFLICT (agent_id, tool_name)
DO UPDATE SET version = EXCLUDED.version,
              description = EXCLUDED.description,
              updated_at = now();
