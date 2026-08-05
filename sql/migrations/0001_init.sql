-- 0001_init.sql
-- Initial schema: servers, agents, commands.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE servers (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    hostname    TEXT NOT NULL,
    environment TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'unknown',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT servers_hostname_environment_key UNIQUE (hostname, environment)
);

CREATE TABLE agents (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    server_id       UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    secret          TEXT NOT NULL,
    version         TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'offline',
    last_heartbeat  TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE commands (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id    UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    tool_name   TEXT NOT NULL,
    payload     JSONB NOT NULL,
    status      TEXT NOT NULL DEFAULT 'pending',
    result      JSONB,
    error       TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_agents_server_id ON agents (server_id);
CREATE INDEX idx_commands_agent_id ON commands (agent_id);
CREATE INDEX idx_commands_status ON commands (status);
