-- 0012_command_audit.sql
-- Immutable audit trail for command origin and approval.
--
-- Every command records who requested it (source + requested_by) and, once an
-- operator approves it, who approved it and when. Source values are 'api'
-- (operator API), 'mcp' (Hermes integration) and 'system' (internal). Existing
-- commands default to 'api' with an empty requester, preserving behaviour.
--
-- The MCP process must never approve its own commands: approval fields are
-- written only by the operator approval endpoint, never by the MCP path.

ALTER TABLE commands
    ADD COLUMN source TEXT NOT NULL DEFAULT 'api',
    ADD COLUMN requested_by TEXT NOT NULL DEFAULT '',
    ADD COLUMN requested_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN approved_by TEXT,
    ADD COLUMN approved_at TIMESTAMPTZ,
    ADD COLUMN approval_note TEXT;

CREATE INDEX idx_commands_source ON commands (source);
