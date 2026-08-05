-- 0007_capability_confirmation.sql
-- Persist each tool's confirmation level with its capability. Read-only
-- tools default to 'none'; write tools advertise 'required'.

ALTER TABLE capabilities
    ADD COLUMN confirmation_level TEXT NOT NULL DEFAULT 'none';
