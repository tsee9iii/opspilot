-- 0003_command_lease.sql
-- Command leasing: track which agent holds a lease on a command.

ALTER TABLE commands
    ADD COLUMN leased_at   TIMESTAMPTZ,
    ADD COLUMN lease_owner TEXT;
