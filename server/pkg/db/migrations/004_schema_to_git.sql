-- 004_schema_to_git.sql
-- Move schema config from DB to git bare repo.
-- DB keeps metadata (name, desc, version) only; content lives in git at schemas/{id}.md.

-- Step 1: Add path column to schemas (nullable during migration)
ALTER TABLE schemas ADD COLUMN path TEXT NOT NULL DEFAULT '';

-- Step 2: Replace active_schema_id with active_schema_path on workspaces
ALTER TABLE workspaces ADD COLUMN active_schema_path TEXT NOT NULL DEFAULT '';

-- Step 3: Drop old active_schema_id column and index
DROP INDEX IF EXISTS idx_workspaces_active_schema;
-- SQLite doesn't support DROP COLUMN easily; we'll handle this in Go migration code
-- For now, active_schema_id becomes vestigial — path is the new canonical reference
