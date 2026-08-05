-- 0009_capability_availability.sql
-- Persist whether each tool is actually executable on the agent that
-- advertised it. Existing capabilities default to available=true.

ALTER TABLE capabilities
    ADD COLUMN available BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN unavailable_reason TEXT NOT NULL DEFAULT '';
