-- name: UpsertCapability :exec
-- Register or refresh one tool capability for an agent.
INSERT INTO capabilities (agent_id, tool_name, version, description, parameter_schema)
VALUES (
    sqlc.arg('agent_id'),
    sqlc.arg('tool_name'),
    sqlc.arg('version'),
    sqlc.arg('description'),
    sqlc.arg('parameter_schema')
)
ON CONFLICT (agent_id, tool_name)
DO UPDATE SET version = EXCLUDED.version,
              description = EXCLUDED.description,
              parameter_schema = EXCLUDED.parameter_schema,
              updated_at = now();
