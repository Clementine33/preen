export interface ApiConfig {
  onUnauthorized?: () => void
  baseUrl: string
  token: string | null
}

/** What the Media Facts Pass is doing, with the files still unknown per library. */
export interface MediaFactsStatus {
  state: 'running' | 'paused' | 'idle'
  /** Libraries with nothing unknown are omitted. */
  libraries: { library_id: string; pending: number; unmeasurable: number }[]
}

/** A reload signal for committed media facts, including the initial connection, or the pass's status. */
export type MediaFactsEvent =
  | {
      /** `works_changed`: a commit may alter what the works pages show; also sent once per connection. */
      type: 'changed' | 'works_changed'
      data: Record<string, never>
    }
  | { type: 'status'; data: MediaFactsStatus }

export interface HealthResponse {
  ok: boolean
  version: string
}

export interface Library {
  id: string
  name: string
  root_path: string
  created_at: string
  updated_at: string
  last_scan_at: string | null
  last_scan_status: string
  last_scan_error: string
  /** Where the media lives; it sets the library's concurrency (docs/api/libraries.md). */
  storage_class: StorageClass
}

export type StorageClass = 'hdd' | 'ssd'

export interface CreateLibraryInput {
  name: string
  root_path: string
}

export interface UpdateLibraryInput {
  name?: string
  root_path?: string
  storage_class?: StorageClass
}

/**
 * One direct child directory of a library root. Its identity is the
 * library-relative path — a rescan cannot renumber what the workbench
 * navigates by — and the audio count is a status fact: a directory without
 * audio is still listed and still browsable.
 */
export interface LibraryDir {
  name: string
  path: string
  rel_path: string
  /** The directory's navigation identity: what a page address carries. */
  dir_id: string
  audio_file_count: number
  file_count: number
  /** When a scan first saw the directory (RFC 3339). */
  added_at: string
  /** The single canonical work number its name carries, else null. */
  work_code: string | null
}

export interface TreeNode {
  name: string
  path: string
  /** Path relative to the member root: what file management addresses. */
  rel_path: string
  type: 'dir' | 'file'
  size?: number
  bitrate: number | null
  format: string
  children?: TreeNode[]
}

/** The member tree read: cached from the inventory, or just re-scanned. */
export interface MemberTreeResponse {
  tree: TreeNode
  /** The identity the route resolved, and the member path it names. */
  dir_id: string
  member_path: string
  /** The work the directory is bound to; null when unbound. */
  rj: string | null
  refreshed?: boolean
}

/** One item of a direct file-management request. */
export interface FileOperationItem {
  source: string
  name?: string
  target_dir?: string
}

export type FileOperation = 'rename' | 'move' | 'soft_delete'

export type FileItemStatus = 'ok' | 'failed' | 'skipped' | 'not_attempted'

export interface FileOperationItemResult {
  source: string
  status: FileItemStatus
  code?: string
  message?: string
  /** Where a rename or move put the item, member-relative. */
  target?: string
  /** Where a soft delete put the item, relative to the library root. */
  recovered_path?: string
}

/** The inventory refresh that follows a write; a failure here is reported, never hidden. */
export interface RefreshState {
  ok: boolean
  code?: string
  message?: string
}

export interface FileOperationResult {
  operation: FileOperation
  member_path: string
  items: FileOperationItemResult[]
  succeeded: number
  failed: number
  untouched: number
  refresh: RefreshState
}

export interface ScanEventData {
  stage: string
  message?: string
  scan_id?: string
  root_path?: string
  files_scanned?: number
  dirs_scanned?: number
  code?: string
}

export type ScanEventType = 'started' | 'progress' | 'completed' | 'cancelled' | 'error'

export interface ScanEvent {
  type: ScanEventType
  data: ScanEventData
}

// ==================== Policy slots ====================

export interface QualitySpec {
  kind: string
  bitrate?: number
}

export interface AudioOutputSpec {
  codec: string
  quality?: QualitySpec
}

export interface PolicyProfile {
  lossless?: AudioOutputSpec | null
  encoded?: AudioOutputSpec | null
}

/**
 * Wire shape of a resolved reconcile policy. no_effect_tags are literal
 * content tags matched case-insensitively as substrings of each Album
 * Root-relative path (a hit → matched / 无音效).
 */
export type ConversionMode = 'strict' | 'available_sources'

export interface ResolvedPolicy {
  schema_version: number
  /** Conversion decision rule; missing = strict (server-side default). */
  mode?: ConversionMode
  other_tags?: string[]
  no_effect_tags: string[]
  matched: PolicyProfile
  unmatched: PolicyProfile
  other?: PolicyProfile
}

/** One of the three fixed global policy slots. policy is null while unconfigured. */
export interface PolicySlot {
  slot: number
  name: string
  policy: ResolvedPolicy | null
  updated_at?: string
}

export interface PolicySlotListResponse {
  slots: PolicySlot[]
}

export interface SavePolicySlotInput {
  name: string
  policy: ResolvedPolicy
}

export interface ClassifierCustomTag {
  id: number
  tag: string
  created_at?: string
}

export interface ClassifierTagLibraryResponse {
  default_no_effect_tags: string[]
  default_other_tags: string[]
  custom_tags: ClassifierCustomTag[]
}

// ==================== Work catalog ====================
// docs/api/works.md#work-catalog. Unknown scalars are null, never zero-values
// (ADR 0009): an unknown adult flag is not false and an unrated work
// is not 0.

/** Server-synced display choices (docs/api/preferences.md). */
export interface UserPreferences {
  /** The ordered Format Preference, uppercase extensions; empty means none. */
  track_formats: string[]
}

export interface WorkUserTag {
  id: string
  name: string
}

/** One listing row: the listing projection, without note text or copies. */
/** The cover file a work already has on disk; bytes come from the content route. */
export interface LocalCover {
  copy_id: string
  rel_path: string
  version: string
}

export interface WorkSummary {
  /** Canonical RJ + 8 digits; the API normalizes 6-digit input itself. */
  rj: string
  title: string | null
  thumbnail_url: string | null
  circle: string | null
  language: string | null
  nsfw: boolean | null
  /** 1 = 全年龄, 2 = R15, 3 = R18; null means not reported. */
  age_category: 1 | 2 | 3 | null
  /** `YYYY-MM-DD`; unknown release dates sort last in either direction. */
  release_date: string | null
  dl_count: number | null
  /** 1–10, or null = unrated. */
  rating: number | null
  favorite: boolean
  user_tags: WorkUserTag[]
  has_note: boolean
  collected_at: string
  copy_count: number
  local_cover?: LocalCover | null
}

/** A voice actor or a source tag, with the source that reported it. */
export interface WorkSourceEntity {
  id: string
  source: string
  source_id: string
  name: string
}

export interface WorkEdition {
  version_root_rj: string
  language: string
  label: string
  edition_id: string
  display_order: number
}

/** What the catalog derived from the evidence, not the evidence itself. */
export interface WorkRelation {
  role: string | null
  language: string | null
  original_rj: string | null
  parent_rj: string | null
}

/** One directory bound to the work; bindings come only from scanning. */
export interface WorkCopy {
  copy_id: string
  library_id: string
  rel_path: string
  /** Navigation id of the copy's folder (the folder page's `dirId`). */
  dir_id: string
}

/** One work number in the edition tree. */
export interface WorkEditionNode {
  rj: string
  role: 'original' | 'edition_page' | 'translation'
  /** A works row exists (relation placeholders included). */
  is_work: boolean
  /** At least one copy in any library right now, online or not. */
  held: boolean
}

/** One language line of the edition tree: JPN, CHI_HANS or CHI_HANT. */
export interface WorkEditionSlot {
  language: string
  label: string | null
  /** The root or any translation under it is held. */
  held: boolean
  root: WorkEditionNode
  /** Every known translation of the page, ordered by rj; empty for JPN. */
  translations: WorkEditionNode[]
}

/** The full work: metadata, personal data, resolved relation and copies. */
export interface WorkDetail extends WorkSummary {
  source_url: string | null
  cover_url: string | null
  sample_cover_url: string | null
  series_id: string | null
  series_name: string | null
  voice_actors: WorkSourceEntity[]
  source_tags: WorkSourceEntity[]
  note: string
  /** Null when the work declares no editions. */
  language_editions: WorkEdition[] | null
  /** Resolved JPN / CHI_HANS / CHI_HANT lines; null when nothing but this work. */
  edition_tree: WorkEditionSlot[] | null
  relation: WorkRelation
  copies: WorkCopy[]
  /** User collections holding the work, in list order; Favorites is `favorite`. */
  collection_ids: string[]
  /** Declared by a relation but never found on disk; absent from the listing. */
  placeholder: boolean
}

export interface WorkListResponse {
  works: WorkSummary[]
  /** How many works the whole filter lists, every page together. */
  total: number
}

/** One work collection (docs/api/collections.md); Favorites is `id: 'favorites'`. */
export interface WorkCollection {
  id: string
  name: string
  work_count: number
  created_at: string
  updated_at: string
}

export interface CollectionListResponse {
  collections: WorkCollection[]
  total: number
}

/** A collection's detail window: the summaries of one page of its members. */
export interface CollectionDetail extends WorkCollection {
  works: WorkSummary[]
  total: number
}

/** The answer of an append: what joined, what was already a member, the new size. */
export interface CollectionAppendResult {
  added: string[]
  skipped: string[]
  work_count: number
}

export interface CollectionWindow {
  limit?: number
  page?: number
  /** `desc` lists the most recently appended members first. */
  direction?: 'asc' | 'desc'
  /** A view sort of the detail route; absent keeps the saved order. */
  sort?: WorksSort
}

/** `rating` puts unrated works last in either direction, as release_date does unknown dates. */
export type WorksSort = 'rj' | 'release_date' | 'collected_at' | 'rating'

/**
 * One filterable source entity (docs/api.md `GET /circles`, `/voice-actors`):
 * `id` is what the listing parameter of the same name takes, and `work_count`
 * counts listed works under the same library scope.
 */
export interface WorkFacet {
  id: string
  name: string
  work_count: number
}

export type WorkFacetKind = 'circle' | 'voice_actor'

/**
 * The set of works a facet list counts over (docs/api.md facet lists): the
 * listing's filters without the order, page size or page number, which describe a
 * page rather than the set.
 */
export type WorksScope = Omit<ListWorksParams, 'sort' | 'direction' | 'page' | 'limit'>

/** Both tag vocabularies under one scope (docs/api.md `GET /tags`): the wall's tag index. */
export interface TagFacets {
  source_tags: WorkFacet[]
  user_tags: WorkFacet[]
}

/**
 * Exactly the parameters the backend accepts (docs/api.md listing filters):
 * anything else is INVALID_ARGUMENT. The three id-list filters each require
 * their `_match` companion when the list is present.
 */
export interface ListWorksParams {
  /** Literal substring of the work number or the title; `%`/`_` are ordinary. */
  q?: string
  /** Comma-separated id lists; a work matches if it has any of them. */
  circle?: string
  language?: string
  library?: string
  voice_actor?: string
  source_tag?: string
  user_tag?: string
  /** Required companions of the id-list filters above. */
  voice_actor_match?: 'all' | 'any'
  source_tag_match?: 'all' | 'any'
  user_tag_match?: 'all' | 'any'
  /**
   * Both vocabularies at once: comma-separated groups of `|`-joined `s:<id>` /
   * `u:<id>` refs. A group is one picked tag (any of its ids); `tag_match`
   * combines the groups and is required with it.
   */
  tag?: string
  tag_match?: 'all' | 'any'
  /** A work that never reported the adult flag. */
  nsfw?: boolean | 'unknown'
  age_category?: 1 | 2 | 3
  /** `YYYY-MM-DD`, inclusive; unknown dates are outside the range. */
  released_from?: string
  released_to?: string
  /** Integer 1–10, inclusive; `unrated: true` unions with the range. */
  rating_from?: number
  rating_to?: number
  unrated?: boolean
  favorite?: boolean
  has_copy?: boolean
  sort?: WorksSort
  direction?: 'asc' | 'desc'
  /** 1–200; an absent limit is 50. */
  limit?: number
  /** 1-based; an absent page is the first, and 0 is refused. */
  page?: number
}

/** The personal data of one work, as the PATCH user-data route answers it. */
export interface WorkUserData {
  rating: number | null
  favorite: boolean
  note: string
}

/**
 * A partial personal-data update (docs/api.md `PATCH /works/{rj}/user-data`):
 * only the fields present are applied, and an explicit `rating: null` clears it.
 */
export interface PatchWorkUserDataParams {
  rating?: number | null
  favorite?: boolean
  note?: string
}

// ==================== Work content manifest ====================
// The detail page's content contract (docs/api.md "Work content manifest").
// Deliberately not shared with the conversion workbench's MemberTree: this
// projection classifies files
// by kind and carries scan-time image dimensions and durations, for different
// consumers at a different change rate.

export type WorkFileKind = 'aud' | 'img' | 'txt' | 'lrc' | 'vid' | 'bin'

export interface WorkContentFile {
  name: string
  /** Copy-root-relative path, '/'-joined. */
  rel_path: string
  kind: WorkFileKind
  /** Bytes. */
  size: number
  /** Audio container: 'MP3' | 'WAV' | … */
  format: string | null
  bitrate: number | null
  /** Image pixels from the header (image.DecodeConfig, ADR 0013); null until measured. */
  width: number | null
  height: number | null
  /** Audio seconds; null until measured after the scan. */
  duration_sec: number | null
  /** Image thumbnail version, the `v` of its immutable thumbnail address (ADR 0013); null for other kinds. */
  thumb_v: string | null
}

/** The identity of one file of one copy: what every content route addresses. */
export interface ContentFileRef {
  rj: string
  copyId: string
  /** Copy-root-relative manifest path, '/'-joined. */
  relPath: string
}

export interface WorkContentDir {
  name: string
  rel_path: string
  children: Array<WorkContentDir | WorkContentFile>
}

export interface WorkContentCopy {
  copy_id: string
  library_id: string
  /** The tree identity of the copy's root directory (ADR 0003), resolved server-side. */
  dir_id: string
  online: boolean
  /** Present only while the member directory resolves on disk (ADR 0013). */
  tree?: WorkContentDir
}

/** One work's file manifest: what the detail page's deck, bay and leaf read. */
export interface WorkContent {
  rj: string
  copies: WorkContentCopy[]
}

// ==================== Worksets and operations ====================

export type PlanningState = 'unplanned' | 'planned' | 'needs_planning' | 'planning' | 'orphaned'
export type GenerationStatus = 'queued' | 'running' | 'completed' | 'failed' | 'canceled' | 'interrupted'
export type OperationType = 'conversion' | 'rename' | 'metadata_refresh'
export const CONVERSION: OperationType = 'conversion'

/** The move destination that names the member root itself (ADR 0002). */
export const MEMBER_ROOT_TARGET = '.'
/** Plans and executes directory renames (ADR 0012). */
export const RENAME: OperationType = 'rename'
export const METADATA_REFRESH: OperationType = 'metadata_refresh'

export interface LibraryRef {
  library_id: string
  name: string
  root_path: string
}

export interface WorksetMember {
  member_id: string
  folder_path: string
  folder_name: string
  /** The durable library-relative path of the member directory. */
  rel_path: string
  /** The navigation identity of that path: what the file page addresses. */
  dir_id: string
}

/** Independent plan facts of one revision; they may overlap by design. */
export interface RevisionCounts {
  members: number
  changed: number
  unmet_targets: number
  blocked: number
  unchanged: number
}

export interface CurrentRevisionSummary {
  plan_id: string
  revision_index: number
  created_at: string
  status: string
  summary_reason: string
  counts: RevisionCounts
  validation_state: string
  stale: boolean | null
}

export interface GenerationProgress {
  generation_id: string
  status: GenerationStatus
  total_roots: number
  completed_roots: number
  current_root: string
  error_count: number
}

export interface GenerationSummary {
  generation_id: string
  status: GenerationStatus
  error_code: string
  error_message: string
  finished_at: string
}

/** Operation-scoped aggregate: the only carrier of planning state. */
export interface Operation {
  workset_id: string
  operation_type: OperationType
  version: number
  planning_state: PlanningState
  current_revision: CurrentRevisionSummary | null
  active_generation: GenerationProgress | null
  latest_generation: GenerationSummary | null
  active_execution: ExecutionProgress | null
  latest_execution: ExecutionRef | null
}

export interface Workset {
  workset_id: string
  title: string
  version: number
  library: LibraryRef | null
  members: WorksetMember[]
  operations: Operation[]
  updated_at: string
  created_at: string
}

export interface WorksetListResponse {
  worksets: Workset[]
  next_cursor?: string
}

/** Why a selected directory did not become a member of the record. */
export type SkippedReason =
  | 'no_audio'
  | 'missing'
  | 'recovery_dir'
  | 'duplicate'
  | 'invalid_path'
  | 'not_work_root'

export interface SkippedFolder {
  path: string
  reason: SkippedReason
}

/** The create-or-replace payload of a library's current record. */
export interface CreateRecordInput {
  folder_paths: string[]
  /** The record the caller saw as current; absent when it saw none. */
  expected_current_id?: string
  title?: string
}

export interface CreateRecordResponse {
  workset: Workset
  created: boolean
  recorded: number
  skipped: SkippedFolder[]
}

/** Zero or one record: a library without one is an ordinary state, not an error. */
export interface CurrentRecordResponse {
  workset: Workset | null
}

export interface ListWorksetsParams {
  library_id?: string
  cursor?: string
  limit?: number
}

// ==================== Operation draft (sparse document) ====================

/** One shared output spec of a desired profile. */
export interface AudioOutputSpec {
  codec: string
  quality?: { kind: string; bitrate?: number }
}

export interface DesiredProfile {
  lossless?: AudioOutputSpec
  encoded?: AudioOutputSpec
}

/** The six override units. What an exception replaces, one group at a time. */
export type OverrideUnit = 'mode' | 'no_effect_tags' | 'other_tags' | 'matched' | 'unmatched' | 'other'

/**
 * A member's explicit values. A unit that is absent inherits the common value;
 * a unit that is present — including an explicitly empty tag array — is a real
 * override that later common changes never touch.
 */
export interface OverrideSet {
  mode?: 'strict' | 'available_sources'
  other_tags?: string[]
  no_effect_tags?: string[]
  matched?: DesiredProfile
  unmatched?: DesiredProfile
  other?: DesiredProfile
}

export interface DraftMember {
  member_id: string
  excluded?: boolean
  overrides?: OverrideSet
}

/** The persisted sparse draft document. */
export interface OperationDraftDocument {
  schema_version: number
  mode?: 'strict' | 'available_sources'
  /**
   * Obsolete-audio handling of the whole operation; absent means soft. It is
   * one global choice (never a per-member override) and is frozen into every
   * revision the draft produces.
   */
  delete_mode?: DeleteMode
  other_tags?: string[]
  no_effect_tags: string[]
  matched: DesiredProfile
  unmatched: DesiredProfile
  other?: DesiredProfile
  members: DraftMember[]
}

/** A member exclusion: the only per-member fact rename and metadata drafts store. */
export interface TaskDraftMember {
  member_id: string
  excluded: boolean
}

/** A rename member record: an exclusion, a literal folder name, or both. */
export interface RenameDraftMember extends TaskDraftMember {
  name?: string
}

export interface RenameDraftDocument {
  schema_version: number
  common: {
    template: string
    strip_bracketed_title?: boolean
    strip_tilde_title?: boolean
    strip_book_title?: boolean
    /** Joins the actors of `{voice_actors}`; absent means "、". */
    voice_actor_separator?: string
  }
  members: RenameDraftMember[]
}

export type MetadataMode = 'fill_missing' | 'refresh_dynamic' | 'refresh_all'
export type MetadataLocale = 'ja-jp' | 'zh-cn' | 'zh-tw'

export interface MetadataDraftDocument {
  schema_version: number
  common: { mode: MetadataMode; locale: MetadataLocale; download_cover: boolean }
  members: TaskDraftMember[]
}

/**
 * The deployment-wide common settings of each Task kind (docs/api/tasks.md#task-settings):
 * the same object a record's draft carries as its common section.
 */
export type ConversionSettings = Pick<
  OperationDraftDocument,
  'mode' | 'delete_mode' | 'other_tags' | 'no_effect_tags' | 'matched' | 'unmatched' | 'other'
>
export type RenameSettings = RenameDraftDocument['common']
export type MetadataSettings = MetadataDraftDocument['common']
export interface TaskSettingsMap {
  conversion: ConversionSettings
  rename: RenameSettings
  metadata_refresh: MetadataSettings
}

/** One Task kind's settings with the ETag the next save must echo as If-Match. */
export interface TaskSettings<T extends OperationType = OperationType> {
  settings: TaskSettingsMap[T]
  etag: string
}

/** Any operation's draft document; the payload belongs to its task (ADR 0004). */
export type AnyDraftDocument = OperationDraftDocument | RenameDraftDocument | MetadataDraftDocument

/** The wire shape of a persisted draft: document wrapped in its task envelope. */
export interface DraftEnvelopeResponse {
  workset_id: string
  operation_type: OperationType
  /** The OPERATION version: echo it back as If-Match on the next save. */
  version: number
  task: TaskEnvelope<OperationDraftDocument>
  updated_at: string
}

/** The unwrapped draft editors consume; the API client maps the envelope to it. */
export interface DraftResponse<D = OperationDraftDocument> {
  workset_id: string
  operation_type: OperationType
  /** The OPERATION version: echo it back as If-Match on the next save. */
  version: number
  schema_version: number
  document: D
  updated_at: string
}

// ==================== Planning sessions ====================

export interface GenerationView {
  generation_id: string
  workset_id: string
  operation_type: OperationType
  status: GenerationStatus
  total_roots: number
  completed_roots: number
  current_root: string
  error_count: number
  revision_id: string
  error_code: string
  error_message: string
  started_at: string
  finished_at: string
  created_at: string
}

export interface RevisionListResponse {
  revisions: CurrentRevisionSummary[]
  /** Keyset cursor for the next (older) page; 0 when the oldest is included. */
  next_before_index?: number
}

export type StartGenerationResponse =
  | { created: true; generation: GenerationView }
  // Unchanged-input replay: current revision stands (200).
  | { created: false; revision: CurrentRevisionSummary }
  // Idempotency-key replay of an already-accepted request: the existing
  // session is returned (202).
  | { created: false; generation: GenerationView }

// Generation SSE events (session_snapshot payload is a GenerationView).
export type GenerationEvent =
  | { type: 'session_snapshot'; data: GenerationView }
  | { type: 'progress'; data: { generation_id: string; total_roots: number; completed_roots: number; current_root: string; error_count: number } }
  | { type: 'completed'; data: { generation_id: string; revision_id: string } }
  | { type: 'failed'; data: { generation_id: string; error_code: string; error_message: string } }
  | { type: 'canceled'; data: { generation_id: string } }
  | { type: 'interrupted'; data: { generation_id: string } }
  | { type: 'error'; data: { code: string; message: string } }

// ==================== Execution sessions ====================

export type DeleteMode = 'soft' | 'hard'
export type ExecutionStatus = 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'interrupted'

/**
 * Task-owned frozen session options. The generic side stores and forwards
 * this payload untouched; the conversion task freezes the delete mode its
 * draft declared.
 */
export interface ExecutionOptions {
  delete_mode?: DeleteMode
}

/** Where a failing component stopped (only set on failure). */
export type ExecutionStage = 'precheck' | 'materialize' | 'validate' | 'commit' | 'remove'

/** Compact session reference attached to the operation and revision views. */
export interface ExecutionRef {
  execution_id: string
  plan_id: string
  status: ExecutionStatus
  error_code?: string
  error_message?: string
  finished_at?: string
}

/** Progress of the session currently queued/running for an operation. */
export interface ExecutionProgress {
  execution_id: string
  plan_id: string
  status: ExecutionStatus
  options?: ExecutionOptions
  total_components: number
  completed_components: number
  total_operations: number
  completed_operations: number
  current_root: string
  current_component_id: string
  current_phase: string
}

/**
 * One component's frozen work and observed outcome. `remaining` lists the
 * operations that did not complete (frozen order) and `recovery` the preserved
 * files an operator may need to look at.
 */
export interface ExecutionComponent {
  component_index: number
  component_id: string
  root_path: string
  partition: 'matched' | 'unmatched'
  status: 'pending' | 'succeeded' | 'failed' | 'canceled'
  stage?: ExecutionStage
  operations: number
  completed_operations: number
  committed: string[]
  removed: string[]
  remaining: string[]
  recovery: string[]
  error_code?: string
  error_message?: string
  inventory_synced: boolean
  inventory_sync_error?: string
  /** Task-owned outcome (metadata refresh: `MetadataTaskResult`). */
  task_result?: unknown
}

/** The session detail payload; also the SSE snapshot. */
export interface ExecutionView {
  execution_id: string
  workset_id: string
  operation_type: OperationType
  plan_id: string
  status: ExecutionStatus
  options?: ExecutionOptions
  total_components: number
  completed_components: number
  total_operations: number
  completed_operations: number
  current_root: string
  current_component_id: string
  current_phase: string
  components: ExecutionComponent[]
  /** The scope this session ran: record-relative paths, absent for a whole
   *  revision. The folders it left out are the next plan's work. */
  selected_folders?: string[]
  error_code: string
  error_message: string
  started_at: string
  finished_at: string
  created_at: string
}

/** The progress event carries counts and the current position, never the
 *  components themselves: those arrive as `component` events, and a client that
 *  missed them reads the detail route. */
export interface ExecutionProgressEvent {
  execution_id: string
  status: ExecutionStatus
  total_components: number
  completed_components: number
  total_operations: number
  completed_operations: number
  current_root: string
  current_component_id: string
  current_phase: string
}

// Execution SSE events (execution_snapshot payload is an ExecutionView).
export type ExecutionEvent =
  | { type: 'execution_snapshot'; data: ExecutionView }
  /** One component's entry, as it appears in `components[]`, sent as its
   *  result lands; the next terminal event always follows the results. */
  | { type: 'component'; data: ExecutionComponent }
  | { type: 'progress'; data: ExecutionProgressEvent }
  | { type: 'succeeded'; data: { execution_id: string; plan_id: string } }
  | { type: 'failed'; data: { execution_id: string; error_code: string; error_message: string } }
  | { type: 'canceled'; data: { execution_id: string } }
  | { type: 'interrupted'; data: { execution_id: string } }
  | { type: 'error'; data: { code: string; message: string } }

export interface StartExecutionResponse {
  /** true only for a fresh 202; false when the idempotency key replayed (200). */
  created: boolean
  execution: ExecutionView
}

// ==================== Revision detail (immutable review) ====================

export interface RootValidation {
  root_index: number
  root_path: string
  root_status: string
  root_error_code: string
  root_error_message: string
  stale: boolean
  inventory_fingerprint: string
  entry_count: number
}

export interface ComponentRootRef {
  step_index: number
  component_index: number
  component_id: string
  root_index: number
}

export interface FileDecision {
  path: string
  resolution: string
  reason_code?: string
  message?: string
  target_path?: string
}

export interface VariantDecision {
  stem: string
  decisions: FileDecision[]
}

export interface LaneDecision {
  lane: string
  decision: string
  reason_code?: string
  message?: string
}

export interface PlanOperation {
  kind: string
  phase: string
  component_id: string
  variant_stem: string
  source_path: string
  target_path?: string
  depends_on?: string[]
}

export interface ComponentOutcome {
  /** Work-relative isolation root; absent/empty is the main scope. */
  scope?: string
  component_id: string
  partition: 'matched' | 'unmatched'
  status: 'ok' | 'blocked'
  reason_code?: string
  message?: string
  lanes: LaneDecision[]
  variant_decisions: VariantDecision[]
  operations: PlanOperation[]
  projected_inventory: string[]
  files: { path: string; size: number; mtime: number }[]
}

export interface StepSummary {
  component_count: number
  blocked_count: number
  operation_count: number
  error_count: number
  /** Stems kept with an unsatisfied target (available_sources mode). */
  unmet_targets?: number
  summary_reason: string
}

export interface ClassifierSnapshot {
  tags?: string[]
  hash?: string
}

/**
 * Opaque task envelope: the generic side names the task kind and its payload
 * schema version; everything inside `payload` belongs to the task.
 */
export interface TaskEnvelope<P> {
  kind: string
  schema_version: number
  payload: P
}

/** The conversion task's reviewable plan payload. */
export interface ConversionPlanPayload {
  policy: unknown
  policy_hash: string
  classifier: ClassifierSnapshot
  summary: StepSummary
  components: ComponentOutcome[]
}

/** One frozen rename proposal. */
export interface RenameComponent {
  member_id: string
  source_name: string
  work_code: string
  raw_name: string
  target_name: string
  action: 'rename' | 'noop' | 'blocked'
  reason_code?: string
  /** The literal name the user gave this folder instead of the template. */
  custom_name?: string
  /** What a hand-renamed folder is called on disk now; the run moves the stored paths to it first. */
  observed_name?: string
  /** The work code a custom name carries when it differs from the bound work. */
  rebind_to?: string
}

export interface CoverTarget {
  copy_id: string
  rj: string
  library_root: string
  rel_path: string
  identity: string
}

/** One frozen metadata refresh target. */
export interface MetadataComponent {
  member_id: string
  target: CoverTarget
  action: 'refresh' | 'skip' | 'cover'
  reason_code?: string
}

/** The plan payload of rename and metadata refresh: one component per member. */
export interface TaskPlanPayload<C> {
  components: C[]
}

export interface MetadataIssue {
  group?: string
  source?: string
  code: string
  message: string
}

/** An executed metadata component's `task_result` (a skip carries no issues). */
export interface MetadataTaskResult {
  rj?: string
  issues?: MetadataIssue[]
}

export type CoverStatus = 'pending' | 'running' | 'succeeded' | 'skipped' | 'failed' | 'canceled'

export interface CoverDelivery {
  id: string
  execution_id: string
  target: CoverTarget
  refresh: boolean
  status: CoverStatus
  error_code?: string
  error_message?: string
}

/** One metadata execution's asynchronous cover deliveries. */
export interface CoverReport {
  items: CoverDelivery[]
  counts: Record<CoverStatus, number>
  downloads: Record<'total' | 'queued' | 'running' | 'done', number>
}

/** One member of a frozen revision: effective settings plus unit provenance. */
export interface RevisionMember {
  member_id: string
  member_name: string
  folder_path: string
  excluded: boolean
  effective: {
    schema_version: number
    mode?: string
    other_tags?: string[]
    no_effect_tags?: string[]
    matched?: DesiredProfile
    unmatched?: DesiredProfile
    other?: DesiredProfile
  }
  /** unit -> 'common' | 'member'. Equal values with different sources stay apart. */
  sources: Record<string, string>
}

export interface RevisionDetailResponse<P = ConversionPlanPayload> {
  plan_id: string
  revision_index: number
  created_at: string
  root_path: string
  snapshot_token: string
  status: string
  summary: {
    operation_count: number
    error_count: number
    total_count: number
    actionable_count: number
    summary_reason: string
  }
  /** The plan snapshot, wrapped in its task envelope. */
  task: TaskEnvelope<P>
  counts: RevisionCounts
  /** Frozen per-member effective settings and inheritance sources. */
  members: RevisionMember[]
  roots: RootValidation[]
  component_roots: ComponentRootRef[]
  /** The session that ran this revision, if any (a revision runs at most once). */
  execution: ExecutionRef | null
}

export interface ApiClientContract {
  createSession(token: string): Promise<void>
  deleteSession(): Promise<void>
  getHealth(signal?: AbortSignal): Promise<HealthResponse>
  listLibraries(signal?: AbortSignal): Promise<Library[]>
  getLibrary(id: string, signal?: AbortSignal): Promise<Library>
  createLibrary(input: CreateLibraryInput): Promise<Library>
  updateLibrary(id: string, input: UpdateLibraryInput): Promise<Library>
  deleteLibrary(id: string): Promise<void>
  scanLibrary(id: string, signal: AbortSignal, rootPath?: string): AsyncIterable<ScanEvent>
  streamMediaFactsEvents(signal: AbortSignal): AsyncIterable<MediaFactsEvent>
  listDirs(libraryId: string, signal?: AbortSignal): Promise<LibraryDir[]>
  getMemberTree(libraryId: string, folderRelPath: string, signal?: AbortSignal): Promise<MemberTreeResponse>
  refreshMemberTree(libraryId: string, folderRelPath: string): Promise<MemberTreeResponse>
  applyFileOperation(
    libraryId: string,
    input: { member_path: string; operation: FileOperation; items: FileOperationItem[] },
  ): Promise<FileOperationResult>
  listPolicySlots(signal?: AbortSignal): Promise<PolicySlot[]>
  savePolicySlot(slot: number, input: SavePolicySlotInput): Promise<PolicySlot>
  listClassifierTags(signal?: AbortSignal): Promise<ClassifierTagLibraryResponse>
  addClassifierTag(tag: string): Promise<ClassifierCustomTag>
  deleteClassifierTag(id: number): Promise<void>
  getCurrentRecord(
    libraryId: string,
    operation: OperationType,
    signal?: AbortSignal,
  ): Promise<CurrentRecordResponse>
  createCurrentRecord(
    libraryId: string,
    operation: OperationType,
    input: CreateRecordInput,
    idempotencyKey: string,
  ): Promise<CreateRecordResponse>
  listWorksets(params?: ListWorksetsParams, signal?: AbortSignal): Promise<WorksetListResponse>
  getWorkset(id: string, signal?: AbortSignal): Promise<Workset>
  listWorks(params?: ListWorksParams, signal?: AbortSignal): Promise<WorkListResponse>
  getWork(rj: string, signal?: AbortSignal): Promise<WorkDetail>
  listCollections(window?: CollectionWindow, signal?: AbortSignal): Promise<CollectionListResponse>
  getCollection(id: string, window?: CollectionWindow, signal?: AbortSignal): Promise<CollectionDetail>
  createCollection(name: string): Promise<WorkCollection>
  renameCollection(id: string, name: string): Promise<WorkCollection>
  deleteCollection(id: string): Promise<void>
  appendToCollection(id: string, rjs: string[]): Promise<CollectionAppendResult>
  removeFromCollection(id: string, rj: string): Promise<void>
  moveCollectionMember(id: string, rj: string, beforeRj: string | null): Promise<void>
  getWorkContent(rj: string, signal?: AbortSignal): Promise<WorkContent>
  workFileUrl(ref: ContentFileRef, version?: string): string
  getWorkText(ref: ContentFileRef, signal?: AbortSignal): Promise<string>
  getWorkThumb(ref: ContentFileRef, height: number, version: string, signal?: AbortSignal): Promise<Blob>
  listWorkFacets(kind: WorkFacetKind, scope?: WorksScope, signal?: AbortSignal): Promise<WorkFacet[]>
  listTagFacets(scope?: WorksScope, signal?: AbortSignal): Promise<TagFacets>
  updateWorkUserData(rj: string, input: PatchWorkUserDataParams): Promise<WorkUserData>
  replaceWorkUserTags(rj: string, tagIds: string[]): Promise<WorkUserTag[]>
  getPreferences(signal?: AbortSignal): Promise<UserPreferences>
  putPreferences(input: UserPreferences): Promise<UserPreferences>
  listUserTags(signal?: AbortSignal): Promise<WorkUserTag[]>
  createUserTag(name: string): Promise<WorkUserTag>
  renameUserTag(id: string, name: string): Promise<WorkUserTag>
  deleteUserTag(id: string): Promise<void>
  getOperation(worksetId: string, operation: OperationType, signal?: AbortSignal): Promise<Operation>
  getOperationDraft(worksetId: string, operation: OperationType, signal?: AbortSignal): Promise<DraftResponse>
  saveOperationDraft(
    worksetId: string,
    operation: OperationType,
    document: AnyDraftDocument,
    ifMatchVersion: number,
  ): Promise<Operation>
  getTaskSettings<T extends OperationType>(operation: T, signal?: AbortSignal): Promise<TaskSettings<T>>
  saveTaskSettings<T extends OperationType>(
    operation: T,
    settings: TaskSettingsMap[T],
    etag: string,
  ): Promise<TaskSettings<T>>
  startGeneration(
    worksetId: string,
    operation: OperationType,
    ifMatchVersion: number,
    idempotencyKey: string,
  ): Promise<StartGenerationResponse>
  getGeneration(
    worksetId: string,
    operation: OperationType,
    generationId: string,
    signal?: AbortSignal,
  ): Promise<GenerationView>
  cancelGeneration(worksetId: string, operation: OperationType, generationId: string): Promise<GenerationView>
  streamGenerationEvents(
    worksetId: string,
    operation: OperationType,
    generationId: string,
    signal: AbortSignal,
  ): AsyncIterable<GenerationEvent>
  getRevision(
    worksetId: string,
    operation: OperationType,
    planId: string,
    signal?: AbortSignal,
  ): Promise<RevisionDetailResponse>
  startExecution(
    worksetId: string,
    operation: OperationType,
    planId: string,
    input: { ifMatchVersion: number; idempotencyKey: string; folderPaths?: string[] },
  ): Promise<StartExecutionResponse>
  getExecution(
    worksetId: string,
    operation: OperationType,
    executionId: string,
    signal?: AbortSignal,
    page?: { from: number; limit: number },
  ): Promise<ExecutionView>
  cancelExecution(
    worksetId: string,
    operation: OperationType,
    executionId: string,
  ): Promise<ExecutionView>
  streamExecutionEvents(
    worksetId: string,
    operation: OperationType,
    executionId: string,
    signal: AbortSignal,
  ): AsyncIterable<ExecutionEvent>
  getExecutionCovers(worksetId: string, executionId: string, signal?: AbortSignal): Promise<CoverReport>
  changeExecutionCovers(worksetId: string, executionId: string, action: 'retry' | 'cancel'): Promise<CoverReport>
}

/** Observed directory location; an interrupted session may contain a recovered success. */
export interface RenameTaskResult {
 source_path: string
 target_path: string
 renamed: boolean
 /** The reason a blocked folder was left alone; the run went on. */
 skipped?: string
 rolled_back?: boolean
 recovered?: boolean
}
