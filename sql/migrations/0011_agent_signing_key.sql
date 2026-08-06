-- 0011_agent_signing_key.sql
-- Per-agent HMAC request signing.
--
-- Each agent is issued a random signing key at registration. The agent uses it
-- to HMAC-SHA256 sign every request; central stores the key so it can verify
-- signatures. The existing Argon2id secret hash is retained at rest for
-- backwards compatibility; it is no longer used for per-request verification.

ALTER TABLE agents
    ADD COLUMN signing_key TEXT NOT NULL DEFAULT '';