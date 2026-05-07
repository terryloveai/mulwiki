-- Migration 002: Workspace active schema + schema derivation tracking

ALTER TABLE workspaces ADD COLUMN active_schema_id TEXT REFERENCES schemas(id);
ALTER TABLE schemas ADD COLUMN derived_from TEXT REFERENCES schemas(id);

CREATE INDEX IF NOT EXISTS idx_workspaces_active_schema ON workspaces(active_schema_id);
CREATE INDEX IF NOT EXISTS idx_schemas_derived_from ON schemas(derived_from);
