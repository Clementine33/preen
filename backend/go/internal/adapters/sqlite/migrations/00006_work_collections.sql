-- +goose Up
CREATE TABLE collections (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
) STRICT;
CREATE TABLE collection_works (
    collection_id TEXT NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    work_id TEXT NOT NULL REFERENCES works(id),
    position INTEGER NOT NULL,
    PRIMARY KEY (collection_id, work_id)
) STRICT;
CREATE INDEX idx_collection_works_order ON collection_works(collection_id, position, work_id);
CREATE INDEX idx_collection_works_work ON collection_works(work_id);
INSERT INTO collections(id,name,created_at,updated_at)
VALUES ('favorites','Favorites','1970-01-01T00:00:00.000000000Z','1970-01-01T00:00:00.000000000Z');

INSERT INTO collection_works(collection_id,work_id,position)
SELECT 'favorites', work_id, row_number() OVER (ORDER BY updated_at, work_id) - 1
FROM work_user_data WHERE favorite = 1;
ALTER TABLE work_user_data DROP COLUMN favorite;
