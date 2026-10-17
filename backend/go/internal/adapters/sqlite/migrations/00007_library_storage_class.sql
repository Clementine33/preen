-- +goose Up
ALTER TABLE libraries ADD COLUMN storage_class TEXT NOT NULL DEFAULT 'hdd' CHECK (storage_class IN ('hdd', 'ssd'));

-- +goose Down
ALTER TABLE libraries DROP COLUMN storage_class;
