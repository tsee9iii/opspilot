-- 0002_agent_auth.sql
-- Agent authentication: registration token management.
--
-- Agent column changes (secret_hash, revoked_at, session_generation) are
-- deferred to a later migration so the current registration flow stays
-- intact.

CREATE TABLE registration_tokens (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash  TEXT NOT NULL UNIQUE,
    environment TEXT,
    expires_at  TIMESTAMPTZ NOT NULL,
    revoked_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
