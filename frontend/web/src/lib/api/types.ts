import type { ClassifierCustomTag, ClassifierTagLibraryResponse, CleanupPreview, CleanupRun, CollectionAppendResult, CollectionDetail, CollectionListResponse, CoverReport, CreateLibraryInput, CreateRecordInput, CreateRecordResponse, CurrentRecordResponse, DraftResponse, ExecutionEvent, ExecutionScope, ExecutionView, FileOperationInput, FileOperationResult, GenerationEvent, GenerationView, HealthResponse, Library, LibraryDir, MediaFactsEvent, MemberReviewResponse, MemberTreeResponse, MetadataDraftDocument, Operation, OperationDraftDocument, OutboundProxy, PatchWorkUserDataParams, PutWorkListeningParams, RenameDraftDocument, RevisionDetailResponse, ScanEvent, StartExecutionResponse, StartGenerationResponse, TagFacets, UpdateCheck, UpdateLibraryInput, UserPreferences, UserPreferencesUpdate, WorkCollection, WorkContent, WorkDetail, WorkFacet, WorkListening, WorkListResponse, Workset, WorksetListResponse, WorkUserData, WorkUserTag } from './views'

import type { WireSchemas } from './wire.gen'

export type * from './views'

export interface ApiConfig {
  onUnauthorized?: () => void
  baseUrl: string
  token: string | null
}

// Closed value sets come from the generated schemas' enums; these names only
// give call sites a short handle on them.
export type StorageClass = WireSchemas['libraryResponse']['storage_class']

export type FileOperation = WireSchemas['fileOperationResponse']['operation']

export type FileItemStatus = WireSchemas['fileOperationItemResponse']['status']

export type ScanMode = NonNullable<WireSchemas['scanRequest']['mode']>

export type ScanEventType = ScanEvent['type']

/**
 * Wire shape of a resolved reconcile policy. no_effect_tags are literal
 * content tags matched case-insensitively as substrings of each Album
 * Root-relative path (a hit → matched / 无音效).
 */
export type ConversionMode = NonNullable<WireSchemas['reconcile_Policy']['mode']>

export type CleanupCategory = WireSchemas['cleanupPreviewResponse']['categories'][number]

export type CleanupStatus = WireSchemas['cleanupRunResponse']['status']

export interface CollectionWindow {
  limit?: number
  page?: number
  /** `desc` lists the most recently appended members first. */
  direction?: 'asc' | 'desc'
  /** A view sort of the detail route; absent keeps the saved order. */
  sort?: WorksSort
}

/**
 * `rating` puts unrated works last in either direction, as release_date does
 * unknown dates; `listened_at` does the same for never-listened works.
 */
export type WorksSort = 'rj' | 'release_date' | 'collected_at' | 'rating' | 'listened_at'

export type WorkFacetKind = 'circle' | 'voice_actor'

/**
 * The set of works a facet list counts over (docs/api.md facet lists): the
 * listing's filters without the order, page size or page number, which describe a
 * page rather than the set.
 */
export type WorksScope = Omit<ListWorksParams, 'sort' | 'direction' | 'page' | 'limit'>

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
  /** Multi-Edition Works: the chain holds two or more language editions. */
  multi_edition?: boolean
  /** Comma-separated `listening`, `finished`, `none`; a work matches any of them. */
  listening?: string
  sort?: WorksSort
  direction?: 'asc' | 'desc'
  /** 1–200; an absent limit is 50. */
  limit?: number
  /** 1-based; an absent page is the first, and 0 is refused. */
  page?: number
}

// ==================== Work content manifest ====================
// The detail page's content contract (docs/api.md "Work content manifest").
// Deliberately not shared with the conversion workbench's MemberTree: this
// projection classifies files
// by kind and carries scan-time image dimensions and durations, for different
// consumers at a different change rate.

export type WorkFileKind = WireSchemas['workContentFileNodeJSON']['kind']

/** The identity of one file of one copy: what every content route addresses. */
export interface ContentFileRef {
  rj: string
  copyId: string
  /** Copy-root-relative manifest path, '/'-joined. */
  relPath: string
}

// ==================== Worksets and operations ====================

export type PlanningState = WireSchemas['operationResponse']['planning_state']

export type GenerationStatus = WireSchemas['generationViewResponse']['status']

export type OperationType = WireSchemas['generationViewResponse']['operation_type']

export const CONVERSION: OperationType = 'conversion'

/** The move destination that names the member root itself (ADR 0002). */
export const MEMBER_ROOT_TARGET = '.'

/** Plans and executes directory renames (ADR 0012). */
export const RENAME: OperationType = 'rename'

export const METADATA_REFRESH: OperationType = 'metadata_refresh'

/** Why a selected directory id did not become a member of the record. */
export type SkippedReason = WireSchemas['skippedFolderResponse']['reason']

export interface ListWorksetsParams {
  library_id?: string
  cursor?: string
  limit?: number
}

/** The six override units. What an exception replaces, one group at a time. */
export type OverrideUnit = 'mode' | 'no_effect_tags' | 'other_tags' | 'matched' | 'unmatched' | 'other'

export type MetadataMode = WireSchemas['refresh_Common']['mode']

export type MetadataLocale = WireSchemas['refresh_Common']['locale']

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

// ==================== Execution sessions ====================

export type DeleteMode = NonNullable<WireSchemas['conversion_ExecutionOptions']['delete_mode']>

export type ExecutionStatus = WireSchemas['executionRefResponse']['status']

/** Where a failing conversion component stopped, or the metadata task's per-component result stage. */
export type ExecutionStage = NonNullable<WireSchemas['executionComponentResponse']['stage']>

export type MemberStatus = WireSchemas['executionMemberSummaryResponse']['status']

/** `view` query value: the summary shape conversion reads. */
export type ExecutionViewMode = 'members'

export type CoverStatus = WireSchemas['coverDestinationResponse']['status']

export interface ApiClientContract {
  createSession: (token: string) => Promise<void>
  deleteSession: () => Promise<void>
  getHealth: (signal?: AbortSignal) => Promise<HealthResponse>
  getUpdateCheck: (signal?: AbortSignal) => Promise<UpdateCheck>
  listLibraries: (signal?: AbortSignal) => Promise<Library[]>
  getLibrary: (id: string, signal?: AbortSignal) => Promise<Library>
  createLibrary: (input: CreateLibraryInput) => Promise<Library>
  updateLibrary: (id: string, input: UpdateLibraryInput) => Promise<Library>
  deleteLibrary: (id: string) => Promise<void>
  scanLibrary: (id: string, signal: AbortSignal, rootPath?: string, mode?: ScanMode) => AsyncIterable<ScanEvent>
  streamMediaFactsEvents: (signal: AbortSignal) => AsyncIterable<MediaFactsEvent>
  listDirs: (libraryId: string, signal?: AbortSignal) => Promise<LibraryDir[]>
  getMemberTree: (libraryId: string, folderRelPath: string, signal?: AbortSignal) => Promise<MemberTreeResponse>
  refreshMemberTree: (libraryId: string, folderRelPath: string) => Promise<MemberTreeResponse>
  applyFileOperation: (
    libraryId: string,
    input: FileOperationInput,
  ) => Promise<FileOperationResult>
  listClassifierTags: (signal?: AbortSignal) => Promise<ClassifierTagLibraryResponse>
  addClassifierTag: (tag: string) => Promise<ClassifierCustomTag>
  deleteClassifierTag: (id: number) => Promise<void>
  getCurrentRecord: (
    libraryId: string,
    operation: OperationType,
    signal?: AbortSignal,
  ) => Promise<CurrentRecordResponse>
  createCurrentRecord: (
    libraryId: string,
    operation: OperationType,
    input: CreateRecordInput,
    idempotencyKey: string,
  ) => Promise<CreateRecordResponse>
  listWorksets: (params?: ListWorksetsParams, signal?: AbortSignal) => Promise<WorksetListResponse>
  getWorkset: (id: string, signal?: AbortSignal) => Promise<Workset>
  listWorks: (params?: ListWorksParams, signal?: AbortSignal) => Promise<WorkListResponse>
  getWork: (rj: string, signal?: AbortSignal) => Promise<WorkDetail>
  listCollections: (window?: CollectionWindow, signal?: AbortSignal) => Promise<CollectionListResponse>
  getCollection: (id: string, window?: CollectionWindow, signal?: AbortSignal) => Promise<CollectionDetail>
  createCollection: (name: string) => Promise<WorkCollection>
  renameCollection: (id: string, name: string) => Promise<WorkCollection>
  deleteCollection: (id: string) => Promise<void>
  appendToCollection: (id: string, rjs: string[]) => Promise<CollectionAppendResult>
  removeFromCollection: (id: string, rj: string) => Promise<void>
  moveCollectionMember: (id: string, rj: string, beforeRj: string | null) => Promise<void>
  getWorkContent: (rj: string, signal?: AbortSignal) => Promise<WorkContent>
  workFileUrl: (ref: ContentFileRef, version?: string) => string
  getWorkText: (ref: ContentFileRef, signal?: AbortSignal) => Promise<string>
  getWorkThumb: (ref: ContentFileRef, height: number, version: string, signal?: AbortSignal) => Promise<Blob>
  workThumbUrl: (ref: ContentFileRef, height: number, version: string) => string
  listWorkFacets: (kind: WorkFacetKind, scope?: WorksScope, signal?: AbortSignal) => Promise<WorkFacet[]>
  listTagFacets: (scope?: WorksScope, signal?: AbortSignal) => Promise<TagFacets>
  updateWorkUserData: (rj: string, input: PatchWorkUserDataParams) => Promise<WorkUserData>
  putWorkListening: (rj: string, input: PutWorkListeningParams, keepalive?: boolean) => Promise<WorkListening>
  deleteWorkListening: (rj: string) => Promise<void>
  replaceWorkUserTags: (rj: string, tagIds: string[]) => Promise<WorkUserTag[]>
  getPreferences: (signal?: AbortSignal) => Promise<UserPreferences>
  putPreferences: (input: UserPreferencesUpdate) => Promise<UserPreferences>
  getOutboundProxy: (signal?: AbortSignal) => Promise<OutboundProxy>
  putOutboundProxy: (input: OutboundProxy) => Promise<OutboundProxy>
  downloadDiagnosticLog: () => Promise<{ blob: Blob, filename: string }>
  previewCleanup: (categories: CleanupCategory[]) => Promise<CleanupPreview>
  startCleanup: (categories: CleanupCategory[]) => Promise<CleanupRun>
  getCleanup: (signal?: AbortSignal) => Promise<CleanupRun | null>
  cancelCleanup: (runId: string) => Promise<CleanupRun>
  listUserTags: (signal?: AbortSignal) => Promise<WorkUserTag[]>
  createUserTag: (name: string) => Promise<WorkUserTag>
  renameUserTag: (id: string, name: string) => Promise<WorkUserTag>
  deleteUserTag: (id: string) => Promise<void>
  getOperation: (worksetId: string, operation: OperationType, signal?: AbortSignal) => Promise<Operation>
  getOperationDraft: (worksetId: string, operation: OperationType, signal?: AbortSignal) => Promise<DraftResponse>
  saveOperationDraft: (
    worksetId: string,
    operation: OperationType,
    document: AnyDraftDocument,
    ifMatchVersion: number,
  ) => Promise<Operation>
  getTaskSettings: <T extends OperationType>(operation: T, signal?: AbortSignal) => Promise<TaskSettings<T>>
  saveTaskSettings: <T extends OperationType>(
    operation: T,
    settings: TaskSettingsMap[T],
    etag: string,
  ) => Promise<TaskSettings<T>>
  startGeneration: (
    worksetId: string,
    operation: OperationType,
    ifMatchVersion: number,
    idempotencyKey: string,
  ) => Promise<StartGenerationResponse>
  getGeneration: (
    worksetId: string,
    operation: OperationType,
    generationId: string,
    signal?: AbortSignal,
  ) => Promise<GenerationView>
  cancelGeneration: (worksetId: string, operation: OperationType, generationId: string) => Promise<GenerationView>
  streamGenerationEvents: (
    worksetId: string,
    operation: OperationType,
    generationId: string,
    signal: AbortSignal,
  ) => AsyncIterable<GenerationEvent>
  getMemberReview: (
    worksetId: string,
    operation: OperationType,
    planId: string,
    memberId: string,
    signal?: AbortSignal,
  ) => Promise<MemberReviewResponse>
  getRevision: (
    worksetId: string,
    operation: OperationType,
    planId: string,
    signal?: AbortSignal,
    /** Only that category's first `limit` member conclusions, in frozen order. */
    category?: { category: string, limit: number },
  ) => Promise<RevisionDetailResponse>
  startExecution: (
    worksetId: string,
    operation: OperationType,
    planId: string,
    input: { ifMatchVersion: number, idempotencyKey: string, scope?: ExecutionScope },
  ) => Promise<StartExecutionResponse>
  getExecution: (
    worksetId: string,
    operation: OperationType,
    executionId: string,
    signal?: AbortSignal,
    read?: { view?: ExecutionViewMode, memberId?: string },
  ) => Promise<ExecutionView>
  cancelExecution: (
    worksetId: string,
    operation: OperationType,
    executionId: string,
  ) => Promise<ExecutionView>
  streamExecutionEvents: (
    worksetId: string,
    operation: OperationType,
    executionId: string,
    signal: AbortSignal,
    view?: ExecutionViewMode,
  ) => AsyncIterable<ExecutionEvent>
  getExecutionCovers: (worksetId: string, executionId: string, signal?: AbortSignal) => Promise<CoverReport>
  changeExecutionCovers: (worksetId: string, executionId: string, action: 'retry' | 'cancel') => Promise<CoverReport>
}
