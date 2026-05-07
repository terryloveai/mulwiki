-- Drop sources and wiki_pages tables.
-- All source content is now stored in per-workspace bare git repos.
-- All wiki content is stored as markdown files with frontmatter in git.

DROP TABLE IF EXISTS sources;
DROP TABLE IF EXISTS wiki_pages;

-- Rename job columns: source_id → source_path, source_ids → source_paths.
-- These now hold git file paths instead of DB UUIDs.
ALTER TABLE jobs RENAME COLUMN source_id TO source_path;
ALTER TABLE jobs RENAME COLUMN source_ids TO source_paths;

-- Same for agent_tasks.
ALTER TABLE agent_tasks RENAME COLUMN source_id TO source_path;
