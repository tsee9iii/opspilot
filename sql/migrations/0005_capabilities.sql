-- 0005_capabilities.sql
-- Agent capabilities: the set of tools an agent exposes to Central.

CREATE TABLE capabilities (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id    UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    tool_name   TEXT NOT NULL,
    version     TEXT NOT NULL,
    description TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT capabilities_agent_id_tool_name_key UNIQUE (agent_id, tool_name)
);

CREATE INDEX idx_capabilities_agent_id ON capabilities (agent_id);
