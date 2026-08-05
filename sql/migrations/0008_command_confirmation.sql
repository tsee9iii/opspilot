-- 0008_command_confirmation.sql
-- Track whether a command's execution has been operator-approved. Write tools
-- (confirmation_level = 'required') produce 'pending' commands that are never
-- leased until approved. Existing commands default to 'approved', so behavior
-- is unchanged.

ALTER TABLE commands
    ADD COLUMN confirmation_status TEXT NOT NULL DEFAULT 'approved',
    ADD COLUMN confirmed_at TIMESTAMPTZ;
