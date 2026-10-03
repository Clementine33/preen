-- +goose Up
ALTER TABLE entries ADD COLUMN is_work_root INTEGER NOT NULL DEFAULT 0;
ALTER TABLE entries_staging ADD COLUMN is_work_root INTEGER NOT NULL DEFAULT 0;
CREATE INDEX idx_entries_work_roots ON entries(root_path, path) WHERE is_work_root = 1;

-- +goose Down
DROP INDEX idx_entries_work_roots;
ALTER TABLE entries_staging DROP COLUMN is_work_root;
ALTER TABLE entries DROP COLUMN is_work_root;
