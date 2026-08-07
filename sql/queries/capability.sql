-- name: UpsertCapability :exec
-- Register or refresh one tool capability for an agent, including whether the
-- tool is currently available on the agent.
INSERT INTO capabilities (agent_id, tool_name, version, description, parameter_schema, confirmation_level, available, unavailable_reason)
VALUES (
    sqlc.arg('agent_id'),
    sqlc.arg('tool_name'),
    sqlc.arg('version'),
    sqlc.arg('description'),
    sqlc.arg('parameter_schema'),
    sqlc.arg('confirmation'),
    sqlc.arg('available'),
    sqlc.arg('unavailable_reason')
)
ON CONFLICT (agent_id, tool_name)
DO UPDATE SET version = EXCLUDED.version,
              description = EXCLUDED.description,
              parameter_schema = EXCLUDED.parameter_schema,
              confirmation_level = EXCLUDED.confirmation_level,
              available = EXCLUDED.available,
              unavailable_reason = EXCLUDED.unavailable_reason,
              updated_at = now();

-- name: GetCapabilityByAgentTool :one
-- Resolve a tool capability for an agent. Used at command creation to decide
-- whether the command may run and whether it requires operator confirmation.
-- Fails closed: a caller must treat pgx.ErrNoRows as "agent has not
-- advertised this tool".
SELECT confirmation_level, available
FROM capabilities
WHERE agent_id = sqlc.arg('agent_id')
  AND tool_name = sqlc.arg('tool_name');
