-- Current development schema. Edit freely; rebuild the development database after changes.
-- Inventory directories own paths; files keep only their directory reference (ADR 0017).
CREATE TABLE inventory_dirs (
    id INTEGER PRIMARY KEY,
    root_path TEXT NOT NULL,
    path TEXT NOT NULL UNIQUE,
    mtime INTEGER NOT NULL DEFAULT 0,
    is_work_root INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER DEFAULT (unixepoch())
);

CREATE TABLE inventory_files (
    dir_ref INTEGER NOT NULL REFERENCES inventory_dirs(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    size INTEGER NOT NULL DEFAULT 0,
    mtime INTEGER NOT NULL DEFAULT 0,
    content_rev INTEGER NOT NULL DEFAULT 1,
    bitrate INTEGER,
    -- Scan-time media facts (ADR 0013), NULL while unmeasured.
    width INTEGER,
    height INTEGER,
    duration_sec REAL,
    format TEXT,
    is_audio INTEGER GENERATED ALWAYS AS (lower(name) LIKE '%.mp3' OR lower(name) LIKE '%.flac' OR lower(name) LIKE '%.wav' OR lower(name) LIKE '%.m4a' OR lower(name) LIKE '%.aac' OR lower(name) LIKE '%.ogg') STORED,
    PRIMARY KEY (dir_ref, name)
) WITHOUT ROWID;

-- Generation credentials: what this app wrote at a path, and the facts that
-- bind the record to those bytes. A row exists only for an output that passed
-- the executor's stream and full-decode verification and was committed, so the
-- row's presence is the verification. A file converted outside this app has no
-- row at all: it is judged by its measured rate where that can judge it, and
-- otherwise it is unconfirmed — never assumed adequate.
CREATE TABLE generation_records (
    path TEXT PRIMARY KEY,
    codec TEXT NOT NULL,
    encoder TEXT NOT NULL,
    encoder_version TEXT NOT NULL,
    bitrate_kbps INTEGER NOT NULL,
    mode TEXT NOT NULL,
    size INTEGER NOT NULL,
    mtime INTEGER NOT NULL,
    content_sha256 TEXT NOT NULL,
    created_at TEXT NOT NULL
);

-- Scan session tracking
CREATE TABLE scan_sessions (
    session_id TEXT PRIMARY KEY,
    root_path TEXT NOT NULL,
    scope_path TEXT,
    kind TEXT NOT NULL DEFAULT 'full',
    status TEXT NOT NULL DEFAULT 'queued',
    error_code TEXT,
    error_message TEXT,
    started_at TEXT DEFAULT CURRENT_TIMESTAMP,
    finished_at TEXT
);

-- Development and release databases have separate lifecycles (ADR 0011).
CREATE TABLE schema_meta (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL DEFAULT ''
);

-- Persisted plans: one immutable snapshot of the current plan of one
-- processing record. Everything task-specific is opaque here: task_kind names
-- the Task that owns the payload and task_schema_version is that payload's
-- schema version. Publishing retires the replaced plan and revokes its cover
-- demand atomically. Retired payload and executions are deleted in bounded
-- background batches; they are never visible through read interfaces. Staging
-- plans belong to their generation and become visible only after promotion.
CREATE TABLE plans (
    plan_id TEXT PRIMARY KEY,
    root_path TEXT NOT NULL,
    scan_root_path TEXT NOT NULL DEFAULT '',
    library_id TEXT REFERENCES libraries(id) ON DELETE SET NULL,
    snapshot_token TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'ready',
    generation_id TEXT REFERENCES plan_generations(generation_id) ON DELETE SET NULL,
    task_kind TEXT NOT NULL DEFAULT '',
    task_schema_version INTEGER NOT NULL DEFAULT 0,
    workset_id TEXT NOT NULL REFERENCES worksets(id) ON DELETE CASCADE,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_plans_generation ON plans(generation_id);
CREATE INDEX idx_plans_staging ON plans(plan_id) WHERE status = 'staging';
CREATE INDEX idx_plans_retired ON plans(plan_id) WHERE status = 'retired';

-- Task payload rows of one plan (owned by the task named in plans.task_kind;
-- the conversion task stores its resolved policy/classifier snapshots here).
-- classifier_pattern stores the task's canonical tag snapshot.
CREATE TABLE conversion_steps (
    plan_id TEXT NOT NULL,
    step_index INTEGER NOT NULL,
    step_type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'ok',
    policy_schema_version INTEGER NOT NULL DEFAULT 0,
    policy_json TEXT NOT NULL DEFAULT '',
    policy_hash TEXT NOT NULL DEFAULT '',
    classifier_pattern TEXT NOT NULL DEFAULT '',
    classifier_hash TEXT NOT NULL DEFAULT '',
    step_summary_json TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (plan_id, step_index),
    FOREIGN KEY (plan_id) REFERENCES plans(plan_id) ON DELETE CASCADE
);

-- Planning roots with their metadata inventory fingerprints. root_status is
-- 'ok' for planned roots and 'missing' for members whose subtree no longer
-- exists at planning time; error_code/message carry the stable root outcome.
CREATE TABLE plan_roots (
    plan_id TEXT NOT NULL,
    root_index INTEGER NOT NULL,
    root_path TEXT NOT NULL,
    root_identity TEXT NOT NULL DEFAULT '',
    inventory_fingerprint TEXT NOT NULL DEFAULT '',
    entry_count INTEGER NOT NULL DEFAULT 0,
    root_status TEXT NOT NULL DEFAULT 'ok',
    root_error_code TEXT NOT NULL DEFAULT '',
    root_error_message TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (plan_id, root_index),
    FOREIGN KEY (plan_id) REFERENCES plans(plan_id) ON DELETE CASCADE
);

-- Member reviews select roots by their frozen member path.
CREATE INDEX idx_plan_roots_path ON plan_roots(plan_id, root_path, root_index);

-- Component outcomes (lanes, decisions, operations, projected inventory)
-- persisted as deterministic JSON snapshots.
CREATE TABLE conversion_components (
    plan_id TEXT NOT NULL,
    step_index INTEGER NOT NULL,
    component_index INTEGER NOT NULL,
    component_id TEXT NOT NULL,
    root_index INTEGER NOT NULL DEFAULT 0,
    partition TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT '',
    reason_code TEXT NOT NULL DEFAULT '',
    outcome_json TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (plan_id, component_index),
    FOREIGN KEY (plan_id) REFERENCES plans(plan_id) ON DELETE CASCADE
);

-- Member review loads complete outcomes only for the selected planning root.
CREATE INDEX idx_conversion_components_root ON conversion_components(plan_id, root_index, component_index);

-- Libraries (web library views)
CREATE TABLE libraries (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL DEFAULT '',
    root_path TEXT NOT NULL DEFAULT '',
    root_path_key TEXT NOT NULL DEFAULT '',
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP,
    last_scan_at TEXT,
    last_scan_status TEXT NOT NULL DEFAULT '',
    last_scan_error TEXT NOT NULL DEFAULT '',
    storage_class TEXT NOT NULL DEFAULT 'hdd' CHECK (storage_class IN ('hdd', 'ssd'))
);

CREATE UNIQUE INDEX idx_libraries_root_path ON libraries(root_path);
CREATE UNIQUE INDEX idx_libraries_root_path_key ON libraries(root_path_key);

-- Worksets: the shared current folder batch of one library. At
-- most one live row per library — the unique index below is the storage-level
-- guarantee, not an application check. A live row with a NULL library_id is
-- orphaned; retired rows stay invisible even after their library is removed.
-- Retirement cleanup removes retained data in bounded batches.
-- Title duplicates are allowed. version is
-- the metadata concurrency counter (rename only); drafts and generations
-- advance their operation's own version (workset_operations.version).
-- creation_idem_key enables replay of a creation request for up to 30 days;
-- creation_request_hash is the canonical hash of the request that owns the
-- key, so a replay with different content is a conflict, not a second record.
CREATE TABLE worksets (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL DEFAULT '',
    library_id TEXT REFERENCES libraries(id) ON DELETE SET NULL,
    root_path TEXT NOT NULL DEFAULT '',
    root_path_key TEXT NOT NULL DEFAULT '',
    version INTEGER NOT NULL DEFAULT 1,
    retired INTEGER NOT NULL DEFAULT 0 CHECK (retired IN (0, 1)),
    creation_idem_key TEXT,
    creation_request_hash TEXT NOT NULL DEFAULT '',
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX idx_worksets_idem ON worksets(creation_idem_key) WHERE creation_idem_key IS NOT NULL;
CREATE UNIQUE INDEX idx_worksets_library
    ON worksets(library_id) WHERE library_id IS NOT NULL AND retired = 0;
CREATE INDEX idx_worksets_library_updated ON worksets(library_id, updated_at);
CREATE INDEX idx_worksets_updated ON worksets(updated_at);

-- Ordered member directories. rel_path (normalized library-relative path) is
-- the durable member identity — the workbench addresses a member by it, and a
-- rescan cannot change it. folder_path/folder_name are display snapshots of
-- the same directory, derived from the root and the relative path.
CREATE TABLE workset_members (
    workset_id TEXT NOT NULL REFERENCES worksets(id) ON DELETE CASCADE,
    member_id TEXT NOT NULL DEFAULT '',
    member_index INTEGER NOT NULL,
    rel_path TEXT NOT NULL,
    folder_path TEXT NOT NULL DEFAULT '',
    folder_name TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (workset_id, member_index),
    UNIQUE (workset_id, rel_path)
);

-- Independent workset.Workset Operations (ADR 0001), keyed by (workset, type).
-- version is the operation concurrency counter: draft saves and revision
-- publication advance it, and If-Match on draft/generation
-- writes is bound to it. current_revision_id is never mutated by a failed,
-- canceled or interrupted generation.
CREATE TABLE workset_operations (
    workset_id TEXT NOT NULL REFERENCES worksets(id) ON DELETE CASCADE,
    operation_type TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    current_revision_id TEXT,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (workset_id, operation_type)
);

-- One sparse operation draft per operation. draft_json holds the four common
-- setting groups plus sparse member overrides; an unoverridden unit is absent
-- rather than materialized from the common value. draft_hash is the canonical
-- content hash used for needs_planning derivation and generation dedup.
CREATE TABLE workset_operation_drafts (
    workset_id TEXT NOT NULL,
    operation_type TEXT NOT NULL,
    schema_version INTEGER NOT NULL DEFAULT 1,
    draft_json TEXT NOT NULL DEFAULT '{}',
    draft_hash TEXT NOT NULL DEFAULT '',
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (workset_id, operation_type),
    FOREIGN KEY (workset_id, operation_type)
        REFERENCES workset_operations(workset_id, operation_type) ON DELETE CASCADE
);

-- Deployment-wide common settings, one row per operation kind (ADR 0008):
-- the payload a global save stores, and what a newly created operation seeds
-- its common settings from. Absent means no global save happened yet, so the
-- task's own seed values stand.
CREATE TABLE common_settings (
    operation_type TEXT PRIMARY KEY,
    settings_json TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL DEFAULT ''
);

-- Immutable operation revisions. revision_index is monotonic within the
-- operation; draft_hash, excluded_scope and draft_snapshot are frozen at
-- generation time (audit + replay + frozen review). operation_version is
-- frozen for audit only (the version is never used as a mutation lock).
CREATE TABLE workset_operation_revisions (
    plan_id TEXT PRIMARY KEY REFERENCES plans(plan_id) ON DELETE CASCADE,
    workset_id TEXT NOT NULL,
    operation_type TEXT NOT NULL,
    revision_index INTEGER NOT NULL,
    draft_hash TEXT NOT NULL DEFAULT '',
    member_hash TEXT NOT NULL DEFAULT '',
    operation_version INTEGER NOT NULL DEFAULT 0,
    excluded_scope TEXT NOT NULL DEFAULT '',
    draft_snapshot TEXT NOT NULL DEFAULT '',
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (workset_id, operation_type, revision_index)
);

-- Async planning session. status: queued|running|completed|failed|canceled|
-- interrupted. idempotency_key is guaranteed for 30 days for completed rows;
-- failed/canceled/interrupted release the key immediately. cancel_requested is
-- observed by the worker at cooperative checkpoints. operation_type scopes the
-- session to one operation: idempotency, the single-active-session rule and
-- SSE/detail addressing all resolve within (workset_id, operation_type).
CREATE TABLE plan_generations (
    generation_id TEXT PRIMARY KEY,
    workset_id TEXT NOT NULL REFERENCES worksets(id) ON DELETE CASCADE,
    operation_type TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'queued',
    idempotency_key TEXT NOT NULL DEFAULT '',
    request_hash TEXT NOT NULL DEFAULT '',
    expected_draft_version INTEGER NOT NULL DEFAULT 0,
    request_json TEXT NOT NULL DEFAULT '',
    total_roots INTEGER NOT NULL DEFAULT 0,
    completed_roots INTEGER NOT NULL DEFAULT 0,
    current_root TEXT NOT NULL DEFAULT '',
    error_count INTEGER NOT NULL DEFAULT 0,
    cancel_requested INTEGER NOT NULL DEFAULT 0,
    revision_id TEXT REFERENCES plans(plan_id) ON DELETE SET NULL,
    error_code TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    started_at TEXT,
    finished_at TEXT,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX idx_plan_generations_op_idem
    ON plan_generations(workset_id, operation_type, idempotency_key)
    WHERE idempotency_key <> '' AND status IN ('queued','running','completed');
CREATE UNIQUE INDEX idx_plan_generations_active_operation
    ON plan_generations(workset_id, operation_type)
    WHERE status IN ('queued','running');
CREATE INDEX idx_plan_generations_op_status
    ON plan_generations(workset_id, operation_type, status, created_at);
-- Worker FIFO compares instants, including timestamps with different offsets.
CREATE INDEX idx_plan_generations_queue
    ON plan_generations(status, julianday(created_at), generation_id);

-- workset.Workset execution sessions: the durable record of one revision
-- being executed (ADR 0001). status: queued|running|succeeded|failed|
-- canceled|interrupted. A revision is executed at most once: the unique
-- idempotency index holds the key for the session's whole life, and the
-- plan_id index answers "has this revision already been executed".
-- This row carries the session's own state only. What each component did is
-- in execution_component_results, committed as that component finishes, so a
-- crash leaves every finished component's facts on disk without the session
-- row ever holding a report that grows with the worklist.
CREATE TABLE plan_executions (
    execution_id TEXT PRIMARY KEY,
    workset_id TEXT NOT NULL REFERENCES worksets(id) ON DELETE CASCADE,
    operation_type TEXT NOT NULL DEFAULT '',
    plan_id TEXT NOT NULL REFERENCES plans(plan_id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'queued',
    idempotency_key TEXT NOT NULL DEFAULT '',
    request_hash TEXT NOT NULL DEFAULT '',
    expected_operation_version INTEGER NOT NULL DEFAULT 0,
    request_json TEXT NOT NULL DEFAULT '',
    total_components INTEGER NOT NULL DEFAULT 0,
    completed_components INTEGER NOT NULL DEFAULT 0,
    total_operations INTEGER NOT NULL DEFAULT 0,
    completed_operations INTEGER NOT NULL DEFAULT 0,
    current_root TEXT NOT NULL DEFAULT '',
    current_component_id TEXT NOT NULL DEFAULT '',
    current_component_index INTEGER NOT NULL DEFAULT 0,
    current_phase TEXT NOT NULL DEFAULT '',
    cancel_requested INTEGER NOT NULL DEFAULT 0,
    error_code TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    started_at TEXT,
    finished_at TEXT,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX idx_plan_executions_op_idem
    ON plan_executions(workset_id, operation_type, idempotency_key)
    WHERE idempotency_key <> '';
-- One execution per revision is a storage invariant, not a check: two
-- concurrent starts would otherwise both pass the read-only eligibility gates.
CREATE UNIQUE INDEX idx_plan_executions_plan_unique ON plan_executions(plan_id);
CREATE INDEX idx_plan_executions_op_status
    ON plan_executions(workset_id, operation_type, status, created_at);
-- Match the execution worker's instant and id tie-break ordering.
CREATE INDEX idx_plan_executions_queue
    ON plan_executions(status, julianday(created_at), execution_id);

-- One component's observed result, written in the boundary transaction that
-- moves the session past it. The parts a component cannot observe for itself —
-- its id, root path, partition and operation count — stay in the session's
-- frozen request; the status and completed_operations columns exist so the
-- session's counters can be recomputed from the rows instead of incremented,
-- which is what makes a replayed boundary write idempotent. result_json holds
-- the result itself (stage, error, committed/removed/remaining/recovery,
-- inventory sync), and a component with no row is pending: nothing has run
-- for it yet.
CREATE TABLE execution_component_results (
    execution_id TEXT NOT NULL REFERENCES plan_executions(execution_id) ON DELETE CASCADE,
    component_index INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    completed_operations INTEGER NOT NULL DEFAULT 0,
    result_json TEXT NOT NULL DEFAULT '{}',
    updated_at TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (execution_id, component_index)
);

-- Global custom classifier tag library. Holds user-entered literal tags for
-- cross-workset reuse. Case-insensitively unique normalized_tag prevents duplicates.
CREATE TABLE classifier_tag_library (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tag TEXT NOT NULL,
    normalized_tag TEXT NOT NULL UNIQUE,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP
);

-- ==================== Work catalog ====================
-- Every table below is STRICT (SQLite >= 3.37): a STRICT table refuses to
-- store a REAL in an INTEGER column, which is what keeps a half-star rating
-- out of an integer column that also carries a range CHECK. Timestamps are
-- written by Go in a fixed-width UTC form (works.TimestampFormat), never by
-- DEFAULT CURRENT_TIMESTAMP, so text order is time order and an index answers
-- a range scan.

-- Logical works. id is the canonical work number (works.WorkCode), which is
-- the immutable identity. Only scanning creates works and records collection time.
CREATE TABLE works (
    id TEXT PRIMARY KEY,
    title TEXT,
    circle_id TEXT REFERENCES circles(id) ON DELETE SET NULL,
    nsfw INTEGER CHECK (nsfw IS NULL OR nsfw IN (0, 1)),
    release_date TEXT CHECK (release_date IS NULL OR release_date GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]'),
    series_id TEXT,
    series_name TEXT,
    source_url TEXT,
    cover_url TEXT,
    sample_cover_url TEXT,
    collected_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    age_category INTEGER CHECK (age_category IS NULL OR age_category IN (1, 2, 3)),
    language TEXT
) STRICT;
CREATE INDEX idx_works_collected ON works(collected_at, id);
CREATE INDEX idx_works_release ON works(release_date, id);
CREATE INDEX idx_works_circle ON works(circle_id);

-- The listing's orderings are answered by indexes, so a page costs a seek
-- rather than a sort of the whole matching set. The trailing id ASC is the
-- tie-break that keeps the order total, and it is ASC in either direction,
-- which is why the descending orderings need an index of their own shape: a
-- reverse scan of the ascending index would reverse the tie-break too. The
-- release-date orderings lead with the null flag, which is what puts unknown
-- release dates last in both directions; the plain idx_works_release above
-- still serves the date-range filter.
CREATE INDEX idx_works_release_asc
    ON works((release_date IS NULL) ASC, release_date ASC, id ASC);
CREATE INDEX idx_works_release_desc
    ON works((release_date IS NULL) ASC, release_date DESC, id ASC);
CREATE INDEX idx_works_collected_desc ON works(collected_at DESC, id ASC);

-- Source entities shared across works. A non-NULL work_id marks an entity
-- identified only within one work: it has no source-assigned id, so its
-- identity key is (source, work_id, name). Without that key a refresh would
-- mint a new row for the same name every time - the entity id would drift
-- (invalidating any filter built on it) and the table would grow forever.
-- The same name never merges across works: work_id is part of the key.
CREATE TABLE circles (
    id TEXT PRIMARY KEY,
    source TEXT NOT NULL,
    source_circle_id TEXT,
    name TEXT NOT NULL,
    work_id TEXT REFERENCES works(id) ON DELETE CASCADE
) STRICT;
CREATE UNIQUE INDEX idx_circles_source_id
    ON circles(source, source_circle_id) WHERE source_circle_id IS NOT NULL;
CREATE UNIQUE INDEX idx_circles_work_scoped
    ON circles(source, work_id, name) WHERE source_circle_id IS NULL AND work_id IS NOT NULL;

CREATE TABLE voice_actors (
    id TEXT PRIMARY KEY,
    source TEXT NOT NULL,
    source_voice_actor_id TEXT,
    name TEXT NOT NULL,
    work_id TEXT REFERENCES works(id) ON DELETE CASCADE
) STRICT;
CREATE UNIQUE INDEX idx_voice_actors_source_id
    ON voice_actors(source, source_voice_actor_id) WHERE source_voice_actor_id IS NOT NULL;
CREATE UNIQUE INDEX idx_voice_actors_work_scoped
    ON voice_actors(source, work_id, name) WHERE source_voice_actor_id IS NULL AND work_id IS NOT NULL;

CREATE TABLE work_voice_actors (
    work_id TEXT NOT NULL REFERENCES works(id) ON DELETE CASCADE,
    voice_actor_id TEXT NOT NULL REFERENCES voice_actors(id) ON DELETE CASCADE,
    position INTEGER NOT NULL CHECK (position >= 0),
    PRIMARY KEY (work_id, voice_actor_id)
) STRICT;

CREATE TABLE tags (
    id TEXT PRIMARY KEY,
    source TEXT NOT NULL,
    source_tag_id TEXT,
    name TEXT NOT NULL,
    work_id TEXT REFERENCES works(id) ON DELETE CASCADE
) STRICT;
CREATE UNIQUE INDEX idx_tags_source_id
    ON tags(source, source_tag_id) WHERE source_tag_id IS NOT NULL;
CREATE UNIQUE INDEX idx_tags_work_scoped
    ON tags(source, work_id, name) WHERE source_tag_id IS NULL AND work_id IS NOT NULL;

CREATE TABLE work_tags (
    work_id TEXT NOT NULL REFERENCES works(id) ON DELETE CASCADE,
    tag_id TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    position INTEGER NOT NULL CHECK (position >= 0),
    PRIMARY KEY (work_id, tag_id)
) STRICT;

-- Sales only: the catalog records what a store sold, and DLsite's dynamic
-- endpoint is the only channel that reports it. That single source is why
-- there is no source column here and no priority rule - only the fetch time
-- comparison in work_source_state. NULL means unknown, which is not zero.
CREATE TABLE work_statistics (
    work_id TEXT PRIMARY KEY REFERENCES works(id) ON DELETE CASCADE,
    dl_count INTEGER CHECK (dl_count IS NULL OR (typeof(dl_count) = 'integer' AND dl_count >= 0))
) STRICT;

-- Which data group came from which source, and when. One row per submitted
-- group. The last CHECK is the invariant that statistics only ever come from
-- DLsite: it is enforced by the storage, not by a handler remembering to.
CREATE TABLE work_source_state (
    work_id TEXT NOT NULL REFERENCES works(id) ON DELETE CASCADE,
    data_group TEXT NOT NULL CHECK (data_group IN ('core','statistics','editions')),
    source TEXT NOT NULL CHECK (source IN ('dlsite','asmr_one')),
    fetched_at TEXT NOT NULL,
    PRIMARY KEY (work_id, data_group),
    CHECK (data_group <> 'statistics' OR source = 'dlsite')
) STRICT;

-- A source report belongs to its canonical subject number, independently of
-- catalog membership. Presence includes an explicit report with no children;
-- source and fetched_at are the sole authority for relation freshness.
CREATE TABLE relation_reports (
    subject_rj TEXT PRIMARY KEY,
    role TEXT NOT NULL CHECK (role IN ('original','parent','child')),
    language TEXT,
    original_rj TEXT,
    parent_rj TEXT,
    source TEXT NOT NULL CHECK (source IN ('dlsite','asmr_one')),
    fetched_at TEXT NOT NULL
) STRICT;

-- Declarations belong to a report; their targets are external numbers.
CREATE TABLE relation_declared_children (
    declaring_rj TEXT NOT NULL REFERENCES relation_reports(subject_rj) ON DELETE CASCADE,
    child_rj TEXT NOT NULL,
    PRIMARY KEY (declaring_rj, child_rj)
) STRICT;

-- Only catalog works own derived rows. Original and parent targets are external
-- numbers: removing their catalog entries does not erase accepted knowledge.
CREATE TABLE work_translations (
    work_id TEXT PRIMARY KEY REFERENCES works(id) ON DELETE CASCADE,
    role TEXT CHECK (role IS NULL OR role IN ('original','parent','child')),
    language TEXT,
    original_work_id TEXT,
    parent_work_id TEXT
) STRICT;
CREATE INDEX idx_work_translations_parent
    ON work_translations(parent_work_id) WHERE parent_work_id IS NOT NULL;

-- The language editions a work declared. version_root_rj is a declared list
-- entry, not a relation edge, so it carries no foreign key.
CREATE TABLE work_language_editions (
    work_id TEXT NOT NULL REFERENCES works(id) ON DELETE CASCADE,
    version_root_rj TEXT NOT NULL,
    language TEXT NOT NULL,
    label TEXT,
    source_edition_id TEXT,
    display_order INTEGER NOT NULL,
    PRIMARY KEY (work_id, language)
) STRICT;

-- Physical copies. Deliberately no foreign key to entries: a rescan deletes
-- inventory rows, and that must never delete a work or its user data.
CREATE TABLE work_copies (
    id TEXT PRIMARY KEY,
    work_id TEXT NOT NULL REFERENCES works(id) ON DELETE CASCADE,
    library_id TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
    rel_path TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE (library_id, rel_path)
) STRICT;
CREATE INDEX idx_work_copies_work ON work_copies(work_id);

-- The single user's data about a work. No row is inserted up front: a missing
-- row reads as unrated with an empty note. Favorites is collection membership.
CREATE TABLE work_user_data (
    work_id TEXT PRIMARY KEY REFERENCES works(id) ON DELETE CASCADE,
    rating INTEGER CHECK (rating IS NULL OR (typeof(rating) = 'integer' AND rating BETWEEN 1 AND 10)),
    note TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL DEFAULT ''
) STRICT;

-- Listening Position per (work, rendition): one snapshot, overwritten. The track
-- fields are null together once the rendition is finished. client and seq order
-- one page's writes; index and count are client snapshots for display only.
CREATE TABLE work_listening (
    work_id TEXT NOT NULL REFERENCES works(id) ON DELETE CASCADE,
    rendition TEXT NOT NULL,
    track_key TEXT,
    position_sec REAL,
    track_index INTEGER,
    track_count INTEGER,
    finished_at TEXT,
    client TEXT NOT NULL,
    seq INTEGER NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (work_id, rendition),
    CHECK ((track_key IS NULL) = (position_sec IS NULL)
       AND (track_key IS NULL) = (track_index IS NULL)
       AND (track_key IS NULL) = (track_count IS NULL))
) STRICT;

CREATE TABLE user_tags (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL
) STRICT;
CREATE UNIQUE INDEX idx_user_tags_name ON user_tags(name);

CREATE TABLE work_user_tags (
    work_id TEXT NOT NULL REFERENCES works(id) ON DELETE CASCADE,
    tag_id TEXT NOT NULL REFERENCES user_tags(id) ON DELETE CASCADE,
    PRIMARY KEY (work_id, tag_id)
) STRICT;

-- Directory paths have a unique index; this index also scopes library subtree ranges.
CREATE INDEX idx_inventory_dirs_root_path ON inventory_dirs(root_path, path);

-- Scan session indexes
CREATE INDEX idx_scan_sessions_root ON scan_sessions(root_path);
CREATE INDEX idx_scan_sessions_status ON scan_sessions(status);

-- workset.Plan indexes
CREATE INDEX idx_plans_root ON plans(root_path);
CREATE INDEX idx_plans_status ON plans(status);
CREATE INDEX idx_plans_workset ON plans(workset_id);
-- The plan payload tables need no plan_id index: their primary keys lead with it.

-- Main-cover deliveries: one row per execution and copy, claimed per work so one download serves them all.
CREATE TABLE cover_deliveries (
 id TEXT PRIMARY KEY,
 execution_id TEXT NOT NULL REFERENCES plan_executions(execution_id) ON DELETE CASCADE,
 work_code TEXT NOT NULL, locale TEXT NOT NULL,
 copy_id TEXT NOT NULL, target_json TEXT NOT NULL, refresh INTEGER NOT NULL,
 status TEXT NOT NULL DEFAULT 'pending', error_code TEXT NOT NULL DEFAULT '', error_message TEXT NOT NULL DEFAULT '',
 published TEXT NOT NULL DEFAULT '', digest TEXT NOT NULL DEFAULT '', UNIQUE(execution_id,copy_id)
) STRICT;
CREATE INDEX cover_deliveries_pending ON cover_deliveries(work_code) WHERE status='pending';

CREATE INDEX idx_inventory_dirs_work_roots ON inventory_dirs(root_path, path) WHERE is_work_root = 1;

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

-- One deployment-wide current/latest manual cleanup summary; never a history ledger.
CREATE TABLE cleanup_run (
 slot INTEGER PRIMARY KEY CHECK (slot = 1),
 summary TEXT NOT NULL CHECK (json_valid(summary))
);
