-- +goose Up

-- Main-cover candidates, shared transfers and independently cancelable copy deliveries.
CREATE TABLE cover_candidates (
 work_code TEXT PRIMARY KEY, url TEXT NOT NULL, backup_url TEXT NOT NULL, version INTEGER NOT NULL
) STRICT;
CREATE TABLE cover_downloads (
 id TEXT PRIMARY KEY, work_code TEXT NOT NULL, version INTEGER NOT NULL,
 url TEXT NOT NULL, backup_url TEXT NOT NULL, locale TEXT NOT NULL, refresh INTEGER NOT NULL,
 status TEXT NOT NULL DEFAULT 'queued', stage_path TEXT NOT NULL DEFAULT '',
 extension TEXT NOT NULL DEFAULT '', digest TEXT NOT NULL DEFAULT '', etag TEXT NOT NULL DEFAULT '',
 last_modified TEXT NOT NULL DEFAULT '', main_checked INTEGER NOT NULL DEFAULT 0, backup_checked INTEGER NOT NULL DEFAULT 0, UNIQUE(work_code,version)
) STRICT;
CREATE INDEX cover_downloads_queue ON cover_downloads(status,id);
CREATE TABLE cover_deliveries (
 id TEXT PRIMARY KEY, job_id TEXT NOT NULL REFERENCES cover_downloads(id) ON DELETE CASCADE,
 execution_id TEXT NOT NULL REFERENCES plan_executions(execution_id) ON DELETE CASCADE,
 copy_id TEXT NOT NULL, target_json TEXT NOT NULL, refresh INTEGER NOT NULL,
 status TEXT NOT NULL DEFAULT 'pending', error_code TEXT NOT NULL DEFAULT '', error_message TEXT NOT NULL DEFAULT '',
 published TEXT NOT NULL DEFAULT '', digest TEXT NOT NULL DEFAULT '', UNIQUE(execution_id,copy_id)
) STRICT;
CREATE INDEX cover_deliveries_execution ON cover_deliveries(execution_id);
CREATE TABLE cover_files (
 copy_id TEXT PRIMARY KEY REFERENCES work_copies(id) ON DELETE CASCADE,
 url TEXT NOT NULL, name TEXT NOT NULL, digest TEXT NOT NULL, etag TEXT NOT NULL, last_modified TEXT NOT NULL
) STRICT;
