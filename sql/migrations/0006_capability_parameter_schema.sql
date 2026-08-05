-- 0006_capability_parameter_schema.sql
-- Persist each tool's parameter schema (JSON Schema document) with its
-- capability. Existing rows fall back to an empty object schema.

ALTER TABLE capabilities
    ADD COLUMN parameter_schema JSONB NOT NULL DEFAULT '{"type":"object","properties":{}}'::jsonb;
