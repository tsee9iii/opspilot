-- name: UpsertCapability :exec
-- Register or refresh one tool capability for an agent.
INSERT INTO capabilities (agent_id, tool_name, version, description, parameter_schema, confirmation_level)
VALUES (
    sqlc.arg('agent_id'),
    sqlc.arg('tool_name'),
    sqlc.arg('version'),
    sqlc.arg('description'),
    sqlc.arg('parameter_schema'),
    sqlc.arg('confirmation')
)
ON CONFLICT (agent_id, tool_name)
DO UPDATE SET version = EXCLUDED.version,
              description = EXCLUDED.description,
              parameter_schema = EXCLUDED.parameter_schema,
              confirmation_level = EXCLUDED.confirmation_level,
              updated_at = now();

-- name: GetCapabilityByAgentTool :one
-- Resolve a tool's confirmation level for an agent. Used at command creation
-- to decide whether the command requires operator confirmation.
SELECT confirmation_level
FROM capabilities
WHERE agent_id = sqlc.arg('agent_id')
  AND tool_name = sqlc.arg('tool_name');
