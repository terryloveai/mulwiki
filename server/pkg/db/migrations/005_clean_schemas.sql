-- 005_clean_schemas.sql
-- Drop vestigial config column from schemas table.
-- Content is stored in git bare repo (schemas/{id}.md), DB keeps metadata only.

ALTER TABLE schemas DROP COLUMN config;
