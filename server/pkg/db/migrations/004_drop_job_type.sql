-- 004_drop_job_type.sql
-- Remove hardcoded job types. All job execution is now schema-driven via agent.

ALTER TABLE jobs DROP COLUMN type;
ALTER TABLE jobs DROP COLUMN source_ids;  -- already unused, cleanup

-- Ensure agent_id is mandatory for new jobs going forward
-- (we don't enforce at DB level since existing rows may have empty values,
-- but the handler should reject empty agent_id)
