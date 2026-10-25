import type { InjectionKey } from 'vue'
import { inject } from 'vue'
import { parseSSEStream, type SSEEvent } from './sse'
import type {
  CollectionAppendResult,
  CollectionDetail,
  CollectionListResponse,
  CollectionWindow,
  WorkCollection,
  TagFacets,
  ApiClientContract,
  AnyDraftDocument,
  ApiConfig,
  ClassifierCustomTag,
  ClassifierTagLibraryResponse,
  CoverReport,
  CreateLibraryInput,
  CreateRecordInput,
  CreateRecordResponse,
  CurrentRecordResponse,
  FileOperation,
  FileOperationItem,
  FileOperationResult,
  LibraryDir,
  MemberTreeResponse,
  MediaFactsEvent,
  DraftEnvelopeResponse,
  DraftResponse,
  ExecutionEvent,
  ExecutionView,
  GenerationEvent,
  GenerationView,
  HealthResponse,
  Library,
  ListWorksParams,
  ListWorksetsParams,
  Operation,
  OperationType,
  RevisionDetailResponse,
  ScanEvent,
  ScanMode,
  StartExecutionResponse,
  StartGenerationResponse,
  UpdateLibraryInput,
  PatchWorkUserDataParams,
  PutWorkListeningParams,
  WorkListening,
  WorkDetail,
  WorkContent,
  ContentFileRef,
  WorkFacet,
  WorkFacetKind,
  WorksScope,
  WorkListResponse,
  WorkUserData,
  OutboundProxy,
  CleanupCategory,
  CleanupPreview,
  CleanupRun,
  UserPreferences,
  WorkUserTag,
  Workset,
  WorksetListResponse,
  TaskSettings,
  TaskSettingsMap,
} from './types'

interface ErrorEnvelope {
  code?: string
  message?: string
  details?: string[]
}

export class ApiError extends Error {
  readonly status: number
  readonly code: string
  /** Machine-readable reason codes (e.g. PLAN_NOT_CONFIRMABLE details). */
  readonly details: string[]

  constructor(status: number, code: string, message: string, details: string[] = []) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.details = details
  }
}

/** The listing's query string: only the parameters that are set ship. */
function worksQuery(params: ListWorksParams): string {
  const query = new URLSearchParams()
  for (const key of [
    'q',
    'circle',
    'language',
    'library',
    'voice_actor',
    'source_tag',
    'user_tag',
    'voice_actor_match',
    'source_tag_match',
    'user_tag_match',
    'tag',
    'tag_match',
    'released_from',
    'released_to',
    'listening',
    'sort',
    'direction',
  ] as const) {
    if (params[key]) query.set(key, params[key])
  }
  for (const key of ['rating_from', 'rating_to', 'age_category', 'limit', 'page'] as const) {
    if (params[key] !== undefined) query.set(key, String(params[key]))
  }
  for (const key of ['nsfw', 'unrated', 'favorite', 'has_copy'] as const) {
    const value = params[key]
    if (value !== undefined) query.set(key, String(value))
  }
  const qs = query.toString()
  return qs ? `?${qs}` : ''
}

function windowQuery(window: CollectionWindow): string {
  const query = new URLSearchParams()
  for (const [key, value] of Object.entries(window)) if (value !== undefined) query.set(key, String(value))
  const qs = query.toString()
  return qs ? `?${qs}` : ''
}

/** Stored conversion JSON may still use the old key; writes only use the new one.
 * Overrides preserve absence, while common settings materialize empty arrays.
 */
function normalizeConversionTags<T extends { no_effect_tags?: string[]; other_tags?: string[] }>(value: T, override = false): T {
  const out = { ...value } as T & { classifier_tags?: string[] }
  if (!override || out.no_effect_tags != null || out.classifier_tags != null) {
    out.no_effect_tags = out.no_effect_tags ?? out.classifier_tags ?? []
  } else {
    delete out.no_effect_tags
  }
  if (!override) out.other_tags = out.other_tags ?? []
  else if (out.other_tags == null) delete out.other_tags
  delete out.classifier_tags
  return out
}

export class ApiClient implements ApiClientContract {
  private readonly baseUrl: string
  private readonly token: string | null
  private readonly onUnauthorized?: () => void

  constructor(config: ApiConfig) {
    this.baseUrl = config.baseUrl.replace(/\/+$/, '')
    this.token = config.token
    this.onUnauthorized = config.onUnauthorized
  }

  createSession(token: string): Promise<void> {
    return this.request('/session', { method: 'POST', body: { token } })
  }

  deleteSession(): Promise<void> {
    return this.request('/session', { method: 'DELETE' })
  }

  getHealth(signal?: AbortSignal): Promise<HealthResponse> {
    return this.request('/health', { signal })
  }

  async listLibraries(signal?: AbortSignal): Promise<Library[]> {
    return (await this.request<{ libraries: Library[] }>('/libraries', { signal })).libraries
  }

  getLibrary(id: string, signal?: AbortSignal): Promise<Library> {
    return this.request(`/libraries/${encodeURIComponent(id)}`, { signal })
  }

  createLibrary(input: CreateLibraryInput): Promise<Library> {
    return this.request('/libraries', { method: 'POST', body: input })
  }

  updateLibrary(id: string, input: UpdateLibraryInput): Promise<Library> {
    return this.request(`/libraries/${encodeURIComponent(id)}`, { method: 'PATCH', body: input })
  }

  deleteLibrary(id: string): Promise<void> {
    return this.request(`/libraries/${encodeURIComponent(id)}`, { method: 'DELETE' })
  }

  async *scanLibrary(id: string, signal: AbortSignal, rootPath?: string, mode?: ScanMode): AsyncGenerator<ScanEvent> {
    const body = { root_path: rootPath, mode }
    const response = await fetch(this.url(`/libraries/${encodeURIComponent(id)}/scans`), {
      method: 'POST',
      headers: this.headers(true),
      body: JSON.stringify(body),
      signal,
    })
    if (!response.ok) throw await this.toApiError(response)
    if (!response.body) {
      throw new ApiError(response.status, 'STREAM_UNAVAILABLE', 'Scan response did not include a stream')
    }
    yield* parseSSEStream<ScanEvent>(response.body)
  }

  async listDirs(libraryId: string, signal?: AbortSignal): Promise<LibraryDir[]> {
    const result = await this.request<{ dirs: LibraryDir[] }>(
      `/libraries/${encodeURIComponent(libraryId)}/dirs`,
      { signal },
    )
    return result.dirs
  }

  /** The stored member tree: what the last scan recorded for this directory. */
  getMemberTree(libraryId: string, dirId: string, signal?: AbortSignal): Promise<MemberTreeResponse> {
    return this.request(
      `/libraries/${encodeURIComponent(libraryId)}/tree?dir=${encodeURIComponent(dirId)}`,
      { signal },
    )
  }

  /**
   * Re-scan one member directory and answer with the refreshed tree. A refresh
   * is a scan: it is refused while a file operation holds the admission slot,
   * and it refuses one in turn.
   */
  refreshMemberTree(libraryId: string, dirId: string): Promise<MemberTreeResponse> {
    return this.request(
      `/libraries/${encodeURIComponent(libraryId)}/tree/refresh?dir=${encodeURIComponent(dirId)}`,
      { method: 'POST', body: {}, timeoutMs: 60_000 },
    )
  }

  /**
   * Direct file management inside one member: single rename, single move, or a
   * batch soft delete. The whole request is refused while a scan, planning
   * session or execution is running.
   */
  applyFileOperation(
    libraryId: string,
    input: { member_path: string; operation: FileOperation; items: FileOperationItem[] },
  ): Promise<FileOperationResult> {
    return this.request(`/libraries/${encodeURIComponent(libraryId)}/file-operations`, {
      method: 'POST',
      body: input,
      timeoutMs: 60_000,
    })
  }

  // ==================== Classifier tag library ====================

  listClassifierTags(signal?: AbortSignal): Promise<ClassifierTagLibraryResponse> {
    return this.request<ClassifierTagLibraryResponse>('/classifier-tags', { signal })
  }

  addClassifierTag(tag: string): Promise<ClassifierCustomTag> {
    return this.request<ClassifierCustomTag>('/classifier-tags', { method: 'POST', body: { tag } })
  }

  deleteClassifierTag(id: number): Promise<void> {
    return this.request(`/classifier-tags/${id}`, { method: 'DELETE' })
  }

  // ==================== Worksets ====================

  /** The library's shared current batch, read through a task entry point. */
  async getCurrentRecord(
    libraryId: string,
    operation: OperationType,
    signal?: AbortSignal,
  ): Promise<CurrentRecordResponse> {
    return this.request(
      `/libraries/${encodeURIComponent(libraryId)}/operations/${encodeURIComponent(operation)}/current`,
      { signal },
    )
  }

  /**
   * Create the library's shared current batch, replacing the
   * record the caller saw as current. The idempotency key makes a retried
   * request answer with the record it already created.
   */
  createCurrentRecord(
    libraryId: string,
    operation: OperationType,
    input: CreateRecordInput,
    idempotencyKey: string,
  ): Promise<CreateRecordResponse> {
    return this.request(
      `/libraries/${encodeURIComponent(libraryId)}/operations/${encodeURIComponent(operation)}/current`,
      {
        method: 'PUT',
        body: input,
        headers: { 'Idempotency-Key': idempotencyKey },
        // The backend re-reads the scanned inventory for the whole selection
        // inside this request.
        timeoutMs: 60_000,
      },
    )
  }

  listWorksets(params: ListWorksetsParams = {}, signal?: AbortSignal): Promise<WorksetListResponse> {
    const query = new URLSearchParams()
    if (params.library_id) query.set('library_id', params.library_id)
    if (params.cursor) query.set('cursor', params.cursor)
    if (params.limit !== undefined) query.set('limit', String(params.limit))
    const qs = query.toString()
    return this.request(`/worksets${qs ? `?${qs}` : ''}`, { signal })
  }

  getWorkset(id: string, signal?: AbortSignal): Promise<Workset> {
    return this.request(`/worksets/${encodeURIComponent(id)}`, { signal })
  }

  // The works listing takes exactly the parameters the backend whitelists
  // (docs/api.md): unknown keys are INVALID_ARGUMENT, so only set ones ship.
  listWorks(params: ListWorksParams = {}, signal?: AbortSignal): Promise<WorkListResponse> {
    return this.request(`/works${worksQuery(params)}`, { signal })
  }

  getWork(rj: string, signal?: AbortSignal): Promise<WorkDetail> {
    // The backend normalizes both 6- and 8-digit spellings to the canonical
    // 8-digit form; the path only ever carries the work number.
    return this.request(`/works/${encodeURIComponent(rj)}`, { signal })
  }

  // Collections (docs/api/collections.md): the list route refuses `direction`,
  // so only the detail window carries it (and `sort`).
  listCollections(window: Omit<CollectionWindow, 'direction' | 'sort'> = {}, signal?: AbortSignal): Promise<CollectionListResponse> {
    return this.request(`/collections${windowQuery(window)}`, { signal })
  }

  getCollection(id: string, window: CollectionWindow = {}, signal?: AbortSignal): Promise<CollectionDetail> {
    return this.request(`/collections/${encodeURIComponent(id)}${windowQuery(window)}`, { signal })
  }

  createCollection(name: string): Promise<WorkCollection> {
    return this.request('/collections', { method: 'POST', body: { name } })
  }

  renameCollection(id: string, name: string): Promise<WorkCollection> {
    return this.request(`/collections/${encodeURIComponent(id)}`, { method: 'PATCH', body: { name } })
  }

  deleteCollection(id: string): Promise<void> {
    return this.request(`/collections/${encodeURIComponent(id)}`, { method: 'DELETE' })
  }

  appendToCollection(id: string, rjs: string[]): Promise<CollectionAppendResult> {
    return this.request(`/collections/${encodeURIComponent(id)}/works`, { method: 'POST', body: { rjs } })
  }

  removeFromCollection(id: string, rj: string): Promise<void> {
    return this.request(`/collections/${encodeURIComponent(id)}/works/${encodeURIComponent(rj)}`, { method: 'DELETE' })
  }

  /** Moves a member before `beforeRj`, or to the end when null. */
  moveCollectionMember(id: string, rj: string, beforeRj: string | null): Promise<void> {
    return this.request(`/collections/${encodeURIComponent(id)}/works/${encodeURIComponent(rj)}`, { method: 'PATCH', body: { before_rj: beforeRj } })
  }

  /** The work's file manifest (docs/api.md "Work content manifest"). */
  getWorkContent(rj: string, signal?: AbortSignal): Promise<WorkContent> {
    return this.request(`/works/${encodeURIComponent(rj)}/content`, { signal })
  }

  /**
   * One manifest file's address for an element that loads it itself (<img>,
   * <audio>): such an element cannot send the bearer header, so the token
   * rides in the address — the one route that accepts it (ADR 0013).
   */
  workFileUrl(ref: ContentFileRef, version?: string): string {
    const params = new URLSearchParams()
    if (this.token) params.set('access_token', this.token)
    if (version) params.set('v', version)
    const qs = params.toString()
    return this.url(contentPath('content', ref) + (qs ? `?${qs}` : ''))
  }

  /**
   * A text file's body, decoded once by the charset the server sniffed and
   * declared (ADR 0013). Response.text() would always decode UTF-8, so the
   * bytes go through a TextDecoder for the declared label.
   */
  async getWorkText(ref: ContentFileRef, signal?: AbortSignal): Promise<string> {
    const response = await fetch(this.url(contentPath('content', ref)), { headers: this.headers(false), signal })
    if (!response.ok) throw await this.toApiError(response)
    const charset = /charset=([^;]+)/i.exec(response.headers.get('Content-Type') ?? '')?.[1]?.trim() ?? 'utf-8'
    const bytes = await response.arrayBuffer()
    try {
      return new TextDecoder(charset).decode(bytes)
    } catch {
      // An unknown label: UTF-8 with replacements beats no text at all.
      return new TextDecoder().decode(bytes)
    }
  }

  /**
   * One image's thumbnail (ADR 0013): fetched with the bearer header so
   * the immutable address carries no token; a full render queue answers 503
   * and the request backs off by Retry-After until it is admitted or aborted.
   */
  async getWorkThumb(ref: ContentFileRef, height: number, version: string, signal?: AbortSignal): Promise<Blob> {
    const path = `${contentPath('thumbs', ref)}?h=${height}&v=${encodeURIComponent(version)}`
    for (;;) {
      const response = await fetch(this.url(path), { headers: this.headers(false), signal })
      if (response.status === 503) {
        await backOff(Number(response.headers.get('Retry-After')) || 1, signal)
        continue
      }
      if (!response.ok) throw await this.toApiError(response)
      return response.blob()
    }
  }

  // The circles or voice actors the works of one listing scope carry; each id
  // is what that listing parameter takes.
  async listWorkFacets(kind: WorkFacetKind, scope: WorksScope = {}, signal?: AbortSignal): Promise<WorkFacet[]> {
    const qs = worksQuery(scope)
    if (kind === 'circle') {
      const body = await this.request<{ circles: WorkFacet[] }>(`/circles${qs}`, { signal })
      return body.circles
    }
    const body = await this.request<{ voice_actors: WorkFacet[] }>(`/voice-actors${qs}`, { signal })
    return body.voice_actors
  }

  /** Both tag vocabularies under one listing scope (docs/api.md `GET /tags`). */
  listTagFacets(scope: WorksScope = {}, signal?: AbortSignal): Promise<TagFacets> {
    return this.request(`/tags${worksQuery(scope)}`, { signal })
  }

  // A partial personal-data update: only the fields present are applied, and
  // an explicit rating null clears it (docs/api.md `PATCH /works/{rj}/user-data`).
  updateWorkUserData(rj: string, input: PatchWorkUserDataParams): Promise<WorkUserData> {
    return this.request(`/works/${encodeURIComponent(rj)}/user-data`, { method: 'PATCH', body: input })
  }

  // One Rendition's Listening Position; keepalive lets the request outlive a
  // closing page (docs/api/works.md `PUT /works/{rj}/listening`).
  putWorkListening(rj: string, input: PutWorkListeningParams, keepalive = false): Promise<WorkListening> {
    return this.request(`/works/${encodeURIComponent(rj)}/listening`, { method: 'PUT', body: input, keepalive })
  }

  deleteWorkListening(rj: string): Promise<void> {
    return this.request(`/works/${encodeURIComponent(rj)}/listening`, { method: 'DELETE' })
  }

  // The work's user tags are exactly the given set after this call; unknown
  // ids answer USER_TAG_NOT_FOUND (docs/api.md `PUT /works/{rj}/user-tags`).
  async replaceWorkUserTags(rj: string, tagIds: string[]): Promise<WorkUserTag[]> {
    const body = await this.request<{ user_tags: WorkUserTag[] }>(`/works/${encodeURIComponent(rj)}/user-tags`, {
      method: 'PUT',
      body: { tag_ids: tagIds },
    })
    return body.user_tags
  }

  // Last write wins; the answer is the normalized list (docs/api/preferences.md).
  getPreferences(signal?: AbortSignal): Promise<UserPreferences> {
    return this.request('/preferences', { signal })
  }

  putPreferences(input: UserPreferences): Promise<UserPreferences> {
    return this.request('/preferences', { method: 'PUT', body: input })
  }

  // Last write wins; the answer is the trimmed URL (docs/api/outbound-proxy.md).
  getOutboundProxy(signal?: AbortSignal): Promise<OutboundProxy> {
    return this.request('/outbound-proxy', { signal })
  }

  putOutboundProxy(input: OutboundProxy): Promise<OutboundProxy> {
    return this.request('/outbound-proxy', { method: 'PUT', body: input })
  }

  // Deployment-wide record cleanup (docs/api/maintenance.md). Start has no
  // request id: a lost answer is resolved by reading the global run, never by resending.
  previewCleanup(categories: CleanupCategory[]): Promise<CleanupPreview> {
    return this.request('/maintenance/cleanup/preview', { method: 'POST', body: { categories } })
  }

  async startCleanup(categories: CleanupCategory[]): Promise<CleanupRun> {
    return (await this.request<{ run: CleanupRun }>('/maintenance/cleanup', { method: 'POST', body: { categories } })).run
  }

  async getCleanup(signal?: AbortSignal): Promise<CleanupRun | null> {
    return (await this.request<{ run: CleanupRun | null }>('/maintenance/cleanup', { signal })).run
  }

  async cancelCleanup(runId: string): Promise<CleanupRun> {
    return (await this.request<{ run: CleanupRun }>(`/maintenance/cleanup/${encodeURIComponent(runId)}/cancel`, { method: 'POST', body: {} })).run
  }

  // Creating by name is idempotent: an existing name answers with the existing
  // tag (docs/api.md `POST /user-tags`).
  async listUserTags(signal?: AbortSignal): Promise<WorkUserTag[]> {
    const body = await this.request<{ user_tags: WorkUserTag[] }>('/user-tags', { signal })
    return body.user_tags
  }

  async createUserTag(name: string): Promise<WorkUserTag> {
    return this.request('/user-tags', { method: 'POST', body: { name } })
  }

  // A name another tag holds answers USER_TAG_EXISTS; tags are never merged.
  renameUserTag(id: string, name: string): Promise<WorkUserTag> {
    return this.request(`/user-tags/${encodeURIComponent(id)}`, { method: 'PATCH', body: { name } })
  }

  // The tag's relations go with it; no work is touched.
  deleteUserTag(id: string): Promise<void> {
    return this.request(`/user-tags/${encodeURIComponent(id)}`, { method: 'DELETE' })
  }

  getOperation(worksetId: string, operation: OperationType, signal?: AbortSignal): Promise<Operation> {
    return this.request(this.operationPath(worksetId, operation), { signal })
  }

  async getOperationDraft(
    worksetId: string,
    operation: OperationType,
    signal?: AbortSignal,
  ): Promise<DraftResponse> {
    const wire = await this.request<DraftEnvelopeResponse>(
      `${this.operationPath(worksetId, operation)}/draft`,
      { signal },
    )
    const payload = wire.task.payload
    return {
      workset_id: wire.workset_id,
      operation_type: wire.operation_type,
      version: wire.version,
      schema_version: wire.task.schema_version,
      // The stored conversion document is sparse: absent keys mean "no
      // records", and the editors rely on the arrays being present. Other
      // tasks reject unknown fields, so their payload passes through as is.
      document:
        operation === 'conversion'
          ? {
              ...normalizeConversionTags(payload),
              members: (payload.members ?? []).map((member) => ({
                ...member,
                ...(member.overrides ? { overrides: normalizeConversionTags(member.overrides, true) } : {}),
              })),
            }
          : payload,
      updated_at: wire.updated_at,
    }
  }

  /** Full replacement of the sparse draft; If-Match is the operation version. */
  saveOperationDraft(
    worksetId: string,
    operation: OperationType,
    document: AnyDraftDocument,
    ifMatchVersion: number,
  ): Promise<Operation> {
    return this.request(`${this.operationPath(worksetId, operation)}/draft`, {
      method: 'PUT',
      body: document,
      headers: { 'If-Match': String(ifMatchVersion) },
    })
  }

  /** A Task kind's deployment-wide settings; works without any record. */
  async getTaskSettings<T extends OperationType>(operation: T, signal?: AbortSignal): Promise<TaskSettings<T>> {
    return this.settingsResult<T>(await this.exchange(`/tasks/${operation}/settings`, { signal }))
  }

  /** The only write path of a Task kind's common settings; If-Match is the ETag read. */
  async saveTaskSettings<T extends OperationType>(
    operation: T,
    settings: TaskSettingsMap[T],
    etag: string,
  ): Promise<TaskSettings<T>> {
    return this.settingsResult<T>(
      await this.exchange(`/tasks/${operation}/settings`, {
        method: 'PUT',
        body: { settings },
        headers: { 'If-Match': etag },
      }),
    )
  }

  private settingsResult<T extends OperationType>(response: { body: unknown; headers: Headers }): TaskSettings<T> {
    const body = response.body as { settings: TaskSettingsMap[T] }
    return { settings: body.settings, etag: response.headers.get('ETag') ?? '' }
  }

  startGeneration(
    worksetId: string,
    operation: OperationType,
    ifMatchVersion: number,
    idempotencyKey: string,
  ): Promise<StartGenerationResponse> {
    // Not a pure enqueue: the backend's dedup fast path recomputes live
    // inventory fingerprints for every member root inside the request, which
    // can exceed the read-path timeout on large worksets. Aborting would
    // surface a false failure AND lose this idempotency key's protection
    // (a retry with a fresh key could start a duplicate session).
    return this.request(`${this.operationPath(worksetId, operation)}/revisions`, {
      method: 'POST',
      headers: { 'If-Match': String(ifMatchVersion), 'Idempotency-Key': idempotencyKey },
      timeoutMs: 60_000,
    })
  }

  getGeneration(
    worksetId: string,
    operation: OperationType,
    generationId: string,
    signal?: AbortSignal,
  ): Promise<GenerationView> {
    return this.request(
      `${this.operationPath(worksetId, operation)}/planning-sessions/${encodeURIComponent(generationId)}`,
      { signal },
    )
  }

  cancelGeneration(worksetId: string, operation: OperationType, generationId: string): Promise<GenerationView> {
    return this.request(
      `${this.operationPath(worksetId, operation)}/planning-sessions/${encodeURIComponent(generationId)}/cancel`,
      { method: 'POST', body: {} },
    )
  }

  async getRevision(
    worksetId: string,
    operation: OperationType,
    planId: string,
    signal?: AbortSignal,
  ): Promise<RevisionDetailResponse> {
    const wire = await this.request<RevisionDetailResponse>(
      `${this.operationPath(worksetId, operation)}/revisions/${encodeURIComponent(planId)}`,
      { signal },
    )
    if (operation === 'conversion' && wire.task.payload.policy) {
      return { ...wire, task: { ...wire.task, payload: { ...wire.task.payload, policy: normalizeConversionTags(wire.task.payload.policy) } } }
    }
    return wire
  }

  /**
   * Enqueue the execution of the operation's current revision, optionally
   * scoped to some of its member folders. The file worklist and the session
   * options are the frozen revision's, never the client's; `folderPaths` says
   * how much of that revision this session covers, and an empty list runs all
   * of it. Disconnecting never cancels the session — only cancelExecution or
   * the backend process lifecycle does.
   */
  startExecution(
    worksetId: string,
    operation: OperationType,
    planId: string,
    input: { ifMatchVersion: number; idempotencyKey: string; folderPaths?: string[] },
  ): Promise<StartExecutionResponse> {
    return this.request(
      `${this.operationPath(worksetId, operation)}/revisions/${encodeURIComponent(planId)}/executions`,
      {
        method: 'POST',
        headers: { 'If-Match': String(input.ifMatchVersion), 'Idempotency-Key': input.idempotencyKey },
        body: { folder_paths: input.folderPaths ?? [] },
      },
    )
  }

  /** Session detail. `page` reads a slice of the component list, which is
   *  ordered by component index; without it the whole list comes back. */
  getExecution(
    worksetId: string,
    operation: OperationType,
    executionId: string,
    signal?: AbortSignal,
    page?: { from: number; limit: number },
  ): Promise<ExecutionView> {
    const query = page
      ? `?components_from=${page.from}&components_limit=${page.limit}`
      : ''
    return this.request(
      `${this.operationPath(worksetId, operation)}/executions/${encodeURIComponent(executionId)}${query}`,
      { signal },
    )
  }

  /** Cooperative cancel; idempotent on terminal sessions (200 either way). */
  cancelExecution(
    worksetId: string,
    operation: OperationType,
    executionId: string,
  ): Promise<ExecutionView> {
    return this.request(
      `${this.operationPath(worksetId, operation)}/executions/${encodeURIComponent(executionId)}/cancel`,
      { method: 'POST', body: {} },
    )
  }

  /** The asynchronous cover deliveries one metadata execution queued. */
  getExecutionCovers(worksetId: string, executionId: string, signal?: AbortSignal): Promise<CoverReport> {
    return this.request(this.coversPath(worksetId, executionId), { signal })
  }

  /** Retry failed/stopped deliveries, or stop pending ones; answers the updated report. */
  changeExecutionCovers(worksetId: string, executionId: string, action: 'retry' | 'cancel'): Promise<CoverReport> {
    return this.request(`${this.coversPath(worksetId, executionId)}/${action}`, { method: 'POST', body: {} })
  }

  private coversPath(worksetId: string, executionId: string): string {
    return `${this.operationPath(worksetId, 'metadata_refresh')}/executions/${encodeURIComponent(executionId)}/covers`
  }

  /** Every mutable planning resource is addressed through its operation. */
  private operationPath(worksetId: string, operation: OperationType): string {
    return `/worksets/${encodeURIComponent(worksetId)}/operations/${encodeURIComponent(operation)}`
  }

  streamGenerationEvents(
    worksetId: string,
    operation: OperationType,
    generationId: string,
    signal: AbortSignal,
  ): AsyncGenerator<GenerationEvent> {
    return this.streamEvents<GenerationEvent>(
      `${this.operationPath(worksetId, operation)}/planning-sessions/${encodeURIComponent(generationId)}/events`,
      signal,
      'Generation events',
    )
  }

  streamMediaFactsEvents(signal: AbortSignal): AsyncGenerator<MediaFactsEvent> {
    return this.streamEvents('/inventory/media-facts/events', signal, 'Media facts events')
  }

  streamExecutionEvents(
    worksetId: string,
    operation: OperationType,
    executionId: string,
    signal: AbortSignal,
  ): AsyncGenerator<ExecutionEvent> {
    return this.streamEvents<ExecutionEvent>(
      `${this.operationPath(worksetId, operation)}/executions/${encodeURIComponent(executionId)}/events`,
      signal,
      'Execution events',
    )
  }

  /** Shared GET-SSE lifecycle: fetch, error envelope, then the parsed stream. */
  private async *streamEvents<T extends SSEEvent>(path: string, signal: AbortSignal, surface: string): AsyncGenerator<T> {
    const response = await fetch(this.url(path), { headers: this.headers(false), signal })
    if (!response.ok) throw await this.toApiError(response)
    if (!response.body) {
      throw new ApiError(response.status, 'STREAM_UNAVAILABLE', `${surface} response did not include a stream`)
    }
    yield* parseSSEStream<T>(response.body)
  }

  private url(path: string): string {
    return `${this.baseUrl}${path}`
  }

  private headers(json = false): Headers {
    const headers = new Headers({ Accept: 'application/json' })
    if (json) headers.set('Content-Type', 'application/json')
    if (this.token) headers.set('Authorization', `Bearer ${this.token}`)
    return headers
  }

  // Safety net for a hung connection (local backend): without it a request
  // that never resolves leaves query UI stuck on an infinite pending state
  // (e.g. the folder-tree page waiting on a never-settling libraries list).
  // The SSE scan stream is not routed through request() and is unaffected.
  // Mutations that trigger long server-side work override this via timeoutMs.
  private static readonly REQUEST_TIMEOUT_MS = 15_000

  private async request<T>(
    path: string,
    options: { method?: string; body?: unknown; signal?: AbortSignal; timeoutMs?: number; headers?: Record<string, string>; keepalive?: boolean } = {},
  ): Promise<T> {
    return (await this.exchange<T>(path, options)).body
  }

  /** request() for the few reads that need a response header too (an ETag). */
  private async exchange<T>(
    path: string,
    options: { method?: string; body?: unknown; signal?: AbortSignal; timeoutMs?: number; headers?: Record<string, string>; keepalive?: boolean } = {},
  ): Promise<{ body: T; headers: Headers }> {
    const controller = new AbortController()
    let removeAbort: (() => void) | null = null
    if (options.signal) {
      if (options.signal.aborted) {
        // Caller already cancelled: still issue the fetch so it rejects
        // immediately with the aborted reason rather than hanging.
        controller.abort(options.signal.reason)
      } else {
        const forwardAbort = () => controller.abort(options.signal?.reason)
        options.signal.addEventListener('abort', forwardAbort, { once: true })
        removeAbort = () => options.signal?.removeEventListener('abort', forwardAbort)
      }
    }
    const timer = setTimeout(
      () => controller.abort(new DOMException('请求超时', 'TimeoutError')),
      options.timeoutMs ?? ApiClient.REQUEST_TIMEOUT_MS,
    )
    try {
      return await this.doRequest<T>(path, options, controller.signal)
    } finally {
      clearTimeout(timer)
      removeAbort?.()
    }
  }

  private async doRequest<T>(
    path: string,
    options: { method?: string; body?: unknown; headers?: Record<string, string>; keepalive?: boolean },
    signal: AbortSignal,
  ): Promise<{ body: T; headers: Headers }> {
    const response = await fetch(this.url(path), {
      method: options.method ?? 'GET',
      headers: { ...Object.fromEntries(this.headers(options.body !== undefined)), ...options.headers },
      body: options.body === undefined ? undefined : JSON.stringify(options.body),
      signal,
      keepalive: options.keepalive,
    })
    if (!response.ok) throw await this.toApiError(response)
    if (response.status === 204) return { body: undefined as T, headers: response.headers }
    return { body: (await response.json()) as T, headers: response.headers }
  }

  private async toApiError(response: Response): Promise<ApiError> {
    if (response.status === 401 && this.token === null) this.onUnauthorized?.()
    let envelope: ErrorEnvelope = {}
    try {
      envelope = (await response.json()) as ErrorEnvelope
    } catch {
      // A proxy can replace the documented JSON envelope; retain useful status context.
    }
    return new ApiError(
      response.status,
      envelope.code ?? 'HTTP_ERROR',
      envelope.message ?? `Request failed with status ${response.status}`,
      envelope.details ?? [],
    )
  }
}

/** A content route's path: each manifest segment escaped on its own. */
function contentPath(route: 'content' | 'thumbs', ref: ContentFileRef): string {
  const segments = ref.relPath.split('/').map(encodeURIComponent).join('/')
  return `/works/${encodeURIComponent(ref.rj)}/${route}/${encodeURIComponent(ref.copyId)}/${segments}`
}

/** Waits the server's Retry-After, ending early with the caller's abort. */
export function backOff(seconds: number, signal?: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    if (signal?.aborted) return reject(signal.reason)
    const onAbort = () => {
      clearTimeout(timer)
      reject(signal?.reason)
    }
    const timer = setTimeout(() => {
      signal?.removeEventListener('abort', onAbort)
      resolve()
    }, seconds * 1000)
    signal?.addEventListener('abort', onAbort, { once: true })
  })
}

export const apiClientKey: InjectionKey<ApiClientContract> = Symbol('api-client')

export function useApiClient(): ApiClientContract {
  const client = inject(apiClientKey)
  if (!client) throw new Error('ApiClient was not provided')
  return client
}
