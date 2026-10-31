import type { QueryClient } from '@tanstack/vue-query'
import type {
  AnyDraftDocument,
  ApiClientContract,
  CoverReport,
  CreateRecordInput,
  CreateRecordResponse,
  ExecutionScope,
  ExecutionView,
  GenerationEvent,
  GenerationView,
  ListWorksetsParams,
  Operation,
  OperationType,
  StartExecutionResponse,
  TaskSettings,
  TaskSettingsMap,
  Workset,
  WorksetListResponse,
} from '@/lib/api/types'
import { infiniteQueryOptions, queryOptions } from '@tanstack/vue-query'
import { executionViewFor } from '@/lib/api/client'
import { errorDetails } from '@/lib/api/error'
import { refreshOrRemoveQueries } from './cache-sync'
import { queryKeys } from './query-keys'

// Workset server-state coordination. All cache writes and invalidations for
// the workset domain live here — pages never touch the QueryClient directly.
//
// Task state carries (worksetId, operationType); every task shares the
// library's current batch key. Replacing the batch drops all of its task state.

const WORKSET_PAGE_SIZE = 50
export function worksetListInfiniteQueryOptions(
  api: ApiClientContract,
  params: { libraryId?: string | null } = {},
) {
  return infiniteQueryOptions({
    queryKey: [
      ...queryKeys.worksets.list(params.libraryId ?? null),
      'infinite',
    ],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }: { pageParam: string | undefined, signal?: AbortSignal }) => {
      const listParams: ListWorksetsParams = { limit: WORKSET_PAGE_SIZE }
      if (params.libraryId)
        listParams.library_id = params.libraryId
      if (pageParam)
        listParams.cursor = pageParam
      return api.listWorksets(listParams, signal)
    },
    getNextPageParam: (lastPage: WorksetListResponse) => lastPage.next_cursor || undefined,
    staleTime: Infinity,
  })
}

// Workset metadata view (title, members, one entry per operation).
export function worksetDetailQueryOptions(api: ApiClientContract, worksetId: string | null | undefined) {
  return queryOptions({
    queryKey: queryKeys.worksets.detail(worksetId ?? ''),
    enabled: Boolean(worksetId),
    staleTime: Infinity,
    queryFn: ({ signal }: { signal?: AbortSignal }) => api.getWorkset(worksetId as string, signal),
  })
}

// One operation's aggregate: version, planning state, current revision and
// session state. This — not the workset — is what the workbench reads.
export function operationQueryOptions(
  api: ApiClientContract,
  worksetId: string | null | undefined,
  operation: OperationType,
) {
  return queryOptions({
    queryKey: queryKeys.worksets.operation(worksetId ?? '', operation),
    enabled: Boolean(worksetId),
    staleTime: Infinity,
    queryFn: ({ signal }: { signal?: AbortSignal }) => api.getOperation(worksetId as string, operation, signal),
  })
}

// The persisted sparse draft document plus the operation version that the next
// save must echo as If-Match.
export function operationDraftQueryOptions(
  api: ApiClientContract,
  worksetId: string | null | undefined,
  operation: OperationType,
) {
  return queryOptions({
    queryKey: queryKeys.worksets.draft(worksetId ?? '', operation),
    enabled: Boolean(worksetId),
    staleTime: Infinity,
    queryFn: ({ signal }: { signal?: AbortSignal }) =>
      api.getOperationDraft(worksetId as string, operation, signal),
  })
}

// A Task kind's deployment-wide common settings with their ETag. They exist
// without any record, so nothing here waits on one.
export function taskSettingsQueryOptions<T extends OperationType>(api: ApiClientContract, operation: T) {
  return queryOptions({
    queryKey: queryKeys.taskSettings.detail(operation),
    staleTime: Infinity,
    queryFn: ({ signal }: { signal?: AbortSignal }) => api.getTaskSettings(operation, signal),
  })
}

/**
 * The library's shared current batch: zero or one, regardless of the task
 * entry point. The workbench reads it without knowing a record id first, and a null answer is an ordinary state, not an error.
 */
export function currentRecordQueryOptions(
  api: ApiClientContract,
  libraryId: string | null | undefined,
  operation: OperationType,
) {
  return queryOptions({
    queryKey: queryKeys.worksets.current(libraryId ?? '', operation),
    enabled: Boolean(libraryId),
    staleTime: Infinity,
    queryFn: ({ signal }: { signal?: AbortSignal }) =>
      api.getCurrentRecord(libraryId as string, operation, signal),
  })
}

/**
 * Create or replace the library's current record. The expected current id is
 * the record the caller saw: a concurrent replace is a conflict instead of an
 * overwrite, and an idempotency key makes a retried request answer with the
 * record it already created. Creation changes which shared batch the library owns, so
 * the old record's caches are dropped once the new one lands.
 */
export function createCurrentRecordMutationOptions(
  api: ApiClientContract,
  queryClient: QueryClient,
  libraryId: string,
  operation: OperationType,
) {
  return {
    mutationFn: (input: { request: CreateRecordInput, idempotencyKey: string }) =>
      api.createCurrentRecord(libraryId, operation, input.request, input.idempotencyKey),
    onSuccess: (result: CreateRecordResponse, input: { request: CreateRecordInput }) => {
      queryClient.setQueryData(queryKeys.worksets.current(libraryId, operation), {
        workset: result.workset,
      })
      const replaced = input.request.expected_current_id
      if (replaced && replaced !== result.workset.workset_id) {
        // The record that was replaced is gone: its detail, draft, sessions and
        // plan are not stale, they no longer exist.
        void refreshOrRemoveQueries(queryClient, queryKeys.worksets.detail(replaced))
        void refreshOrRemoveQueries(queryClient, ['worksets', 'operation', replaced])
        void refreshOrRemoveQueries(queryClient, ['worksets', 'draft', replaced])
        void refreshOrRemoveQueries(queryClient, ['worksets', 'revisions', replaced])
        void refreshOrRemoveQueries(queryClient, ['worksets', 'executions', replaced])
      }
      return result
    },
  }
}

// Frozen plan summary with live member input facts. Revisit stale input even
// when the plan id is unchanged and the global mount policy disables refresh.
export function operationRevisionDetailQueryOptions(
  api: ApiClientContract,
  worksetId: string | null | undefined,
  operation: OperationType,
  planId: string | null | undefined,
) {
  return queryOptions({
    queryKey: queryKeys.worksets.revision(worksetId ?? '', operation, planId ?? ''),
    enabled: Boolean(worksetId && planId),
    staleTime: 60_000,
    refetchOnMount: true,
    queryFn: ({ signal }: { signal?: AbortSignal }) =>
      api.getRevision(worksetId as string, operation, planId as string, signal),
  })
}

/**
 * One Task-defined category of a revision's member conclusions, the first
 * `limit` in frozen order; every read also carries `category_counts` and the
 * session ref, so a page that needs only those reads a one-member category.
 * Keyed under the revision so its invalidation covers these reads. Keeps the
 * previous read of the same plan while another category loads.
 */
export function operationRevisionCategoryQueryOptions(
  api: ApiClientContract,
  worksetId: string | null | undefined,
  operation: OperationType,
  planId: string | null | undefined,
  category: string,
  limit: number,
) {
  return queryOptions({
    queryKey: [
      ...queryKeys.worksets.revision(worksetId ?? '', operation, planId ?? ''),
      'category',
      category,
      limit,
    ] as const,
    enabled: Boolean(worksetId && planId),
    staleTime: 60_000,
    refetchOnMount: true,
    placeholderData: previous => (previous?.plan_id === planId
      ? previous
      : undefined),
    queryFn: ({ signal }: { signal?: AbortSignal }) =>
      api.getRevision(worksetId as string, operation, planId as string, signal, { category, limit }),
  })
}

// Member input validity is live even while its frozen plan id stays unchanged.
export function operationMemberReviewQueryOptions(
  api: ApiClientContract,
  queryClient: QueryClient,
  worksetId: string | null | undefined,
  operation: OperationType,
  planId: string | null | undefined,
  memberId: string | null | undefined,
) {
  return queryOptions({
    queryKey: queryKeys.worksets.memberReview(worksetId ?? '', operation, planId ?? '', memberId ?? ''),
    enabled: Boolean(worksetId && planId && memberId),
    staleTime: 60_000,
    refetchOnMount: true,
    retry: false,
    queryFn: async ({ signal }: { signal?: AbortSignal }) => {
      try {
        return await api.getMemberReview(worksetId as string, operation, planId as string, memberId as string, signal)
      }
      catch (error) {
        if (errorDetails(error).code === 'REVISION_NOT_FOUND') {
          // Publication replaced this plan between the operation and member reads.
          // Its observers follow the refreshed operation's current plan id.
          await queryClient.invalidateQueries({
            queryKey: queryKeys.worksets.operation(worksetId as string, operation),
            exact: true,
          })
        }
        throw error
      }
    },
  })
}

// Global classifier tag library (defaults from config + custom from DB).
export function classifierTagLibraryQueryOptions(api: ApiClientContract) {
  return queryOptions({
    queryKey: queryKeys.classifierTags.list(),
    staleTime: Infinity,
    queryFn: ({ signal }: { signal?: AbortSignal }) => api.listClassifierTags(signal),
  })
}

export function addClassifierTagMutationOptions(api: ApiClientContract, queryClient: QueryClient) {
  return {
    mutationFn: (tag: string) => api.addClassifierTag(tag),
    onSuccess: () => {
      void refreshOrRemoveQueries(queryClient, queryKeys.classifierTags.list())
    },
  }
}

export function deleteClassifierTagMutationOptions(api: ApiClientContract, queryClient: QueryClient) {
  return {
    mutationFn: (id: number) => api.deleteClassifierTag(id),
    onSuccess: () => {
      void refreshOrRemoveQueries(queryClient, queryKeys.classifierTags.list())
    },
  }
}

// ==================== Mutations ====================

/**
 * Save the full sparse draft document. The common section is not written here
 * (the backend refuses a differing one): member records only.
 */
export function saveOperationDraftMutationOptions(api: ApiClientContract, queryClient: QueryClient) {
  return {
    mutationFn: (input: {
      worksetId: string
      operation: OperationType
      document: AnyDraftDocument
      ifMatchVersion: number
    }) => api.saveOperationDraft(input.worksetId, input.operation, input.document, input.ifMatchVersion),
    onSuccess: () => refreshOrRemoveQueries(queryClient, queryKeys.worksets.all()),
  }
}

/**
 * Save a Task kind's common settings, the only write path of them. The backend
 * stores them deployment-wide and rewrites every editable record's draft of
 * the kind in the same transaction, so the whole workset domain is refreshed:
 * each record reads the normalized document and the version it now has
 * (ADR 0008). The saved settings replace the cached ones directly, carrying
 * the new ETag.
 */
export function saveTaskSettingsMutationOptions<T extends OperationType>(
  api: ApiClientContract,
  queryClient: QueryClient,
  operation: T,
) {
  return {
    mutationFn: (input: { settings: TaskSettingsMap[T], etag: string }) =>
      api.saveTaskSettings(operation, input.settings, input.etag),
    onSuccess: (saved: TaskSettings<T>) => {
      queryClient.setQueryData(queryKeys.taskSettings.detail(operation), saved)
      return refreshOrRemoveQueries(queryClient, queryKeys.worksets.all())
    },
  }
}

export function startGenerationMutationOptions(api: ApiClientContract, queryClient: QueryClient) {
  return {
    mutationFn: (input: {
      worksetId: string
      operation: OperationType
      ifMatchVersion: number
      idempotencyKey: string
    }) => api.startGeneration(input.worksetId, input.operation, input.ifMatchVersion, input.idempotencyKey),
    onSuccess: (result: { created: boolean }, input: { worksetId: string, operation: OperationType }) => {
      if (result.created) {
        // 202: the generation SSE composable owns follow-up sync from here.
        void refreshOrRemoveQueries(queryClient, queryKeys.worksets.operation(input.worksetId, input.operation))
      }
      else {
        // created:false replay — input unchanged, current revision stands.
        void Promise.all([
          refreshOrRemoveQueries(queryClient, queryKeys.worksets.operation(input.worksetId, input.operation)),
          refreshOrRemoveQueries(queryClient, queryKeys.worksets.revisionsPrefix(input.worksetId, input.operation)),
        ])
      }
    },
  }
}

// Exact-session detail is shared by recovery and any mounted task view.
export function generationQueryOptions(api: ApiClientContract, worksetId: string, operation: OperationType, generationId: string) {
  return queryOptions({
    queryKey: queryKeys.worksets.generation(worksetId, operation, generationId),
    staleTime: 0,
    retry: false,
    queryFn: ({ signal }) => api.getGeneration(worksetId, operation, generationId, signal),
  })
}

export function syncGenerationSnapshot(queryClient: QueryClient, session: GenerationView) {
  queryClient.setQueryData(queryKeys.worksets.generation(session.workset_id, session.operation_type, session.generation_id), session)
  queryClient.setQueryData<Operation>(queryKeys.worksets.operation(session.workset_id, session.operation_type), (previous) => {
    if (!previous || previous.active_generation?.generation_id !== session.generation_id)
      return previous
    return {
      ...previous,
      active_generation: session.status === 'queued' || session.status === 'running'
        ? session
        : null,
      latest_generation: session,
    }
  })
}

export function syncGenerationEvent(queryClient: QueryClient, worksetId: string, operation: OperationType, generationId: string, event: GenerationEvent) {
  if (event.type === 'session_snapshot') {
    syncGenerationSnapshot(queryClient, event.data)
  }
  else if (event.type !== 'error') {
    const previous = queryClient.getQueryData<GenerationView>(queryKeys.worksets.generation(worksetId, operation, generationId))
    if (previous) {
      syncGenerationSnapshot(queryClient, {
        ...previous,
        ...event.data,
        status: event.type === 'progress'
          ? previous.status
          : event.type,
      })
    }
  }
}

export function cancelGenerationMutationOptions(api: ApiClientContract) {
  return {
    mutationFn: (input: { worksetId: string, operation: OperationType, generationId: string }) =>
      api.cancelGeneration(input.worksetId, input.operation, input.generationId),
  }
}

// ==================== Execution sessions ====================

// One session's authoritative detail. The SSE stream seeds this same entry with
// its snapshot and keeps the open panel current through component (or member)
// events; the GET stays the fallback (a missed terminal event, a reconnect).
// Every operation reads the per-member summary shape.
export function executionQueryOptions(
  api: ApiClientContract,
  worksetId: string | null | undefined,
  operation: OperationType,
  executionId: string | null | undefined,
) {
  return queryOptions({
    queryKey: queryKeys.worksets.execution(worksetId ?? '', operation, executionId ?? ''),
    enabled: Boolean(worksetId && executionId),
    staleTime: 0,
    retry: false,
    queryFn: ({ signal }: { signal?: AbortSignal }) =>
      api.getExecution(worksetId as string, operation, executionId as string, signal, {
        view: executionViewFor(operation),
      }),
  })
}

/**
 * One member's Member Result (its components in full). Keyed under the session
 * so the terminal sweep of the executions prefix re-reads it once.
 */
export function executionMemberQueryOptions(
  api: ApiClientContract,
  worksetId: string | null | undefined,
  operation: OperationType,
  executionId: string | null | undefined,
  memberId: string | null | undefined,
) {
  return queryOptions({
    queryKey: [
      ...queryKeys.worksets.execution(worksetId ?? '', operation, executionId ?? ''),
      'member',
      memberId ?? '',
    ] as const,
    enabled: Boolean(worksetId && executionId && memberId),
    staleTime: 0,
    retry: false,
    queryFn: ({ signal }: { signal?: AbortSignal }) =>
      api.getExecution(worksetId as string, operation, executionId as string, signal, { memberId: memberId as string }),
  })
}

// Exact-session progress is shared by polling and the visited operation cache.
export function syncExecutionSnapshot(queryClient: QueryClient, session: ExecutionView) {
  queryClient.setQueryData(queryKeys.worksets.execution(session.workset_id, session.operation_type, session.execution_id), session)
  queryClient.setQueryData<Operation>(queryKeys.worksets.operation(session.workset_id, session.operation_type), (previous) => {
    if (!previous || previous.active_execution?.execution_id !== session.execution_id)
      return previous
    return {
      ...previous,
      active_execution: session.status === 'queued' || session.status === 'running'
        ? session
        : null,
      latest_execution: session,
    }
  })
}

/**
 * Enqueue the execution of the current revision, optionally scoped to some of
 * its folders. The server re-checks every eligibility fact inside the request;
 * a refusal is explained, never retried with a fresh key (a different key would
 * be a second run attempt).
 */
export function startExecutionMutationOptions(api: ApiClientContract, queryClient: QueryClient) {
  return {
    mutationFn: (input: {
      worksetId: string
      operation: OperationType
      planId: string
      ifMatchVersion: number
      idempotencyKey: string
      /** The selected members; absent runs the whole revision. */
      scope?: ExecutionScope
    }) =>
      api.startExecution(input.worksetId, input.operation, input.planId, {
        ifMatchVersion: input.ifMatchVersion,
        idempotencyKey: input.idempotencyKey,
        scope: input.scope,
      }),
    onSuccess: (
      result: StartExecutionResponse,
      input: { worksetId: string, operation: OperationType, planId: string },
    ) => {
      queryClient.setQueryData(
        queryKeys.worksets.execution(input.worksetId, input.operation, result.execution.execution_id),
        result.execution,
      )
      void Promise.all([
        refreshOrRemoveQueries(queryClient, queryKeys.worksets.operation(input.worksetId, input.operation)),
        refreshOrRemoveQueries(
          queryClient,
          queryKeys.worksets.revision(input.worksetId, input.operation, input.planId),
        ),
      ])
    },
  }
}

// The cancel response is the authoritative outcome (the server answers with
// the session either way): it seeds the report entry so a client whose SSE
// died still ends on the real state.
export function cancelExecutionMutationOptions(api: ApiClientContract, queryClient: QueryClient) {
  return {
    mutationFn: (input: { worksetId: string, operation: OperationType, executionId: string }) =>
      api.cancelExecution(input.worksetId, input.operation, input.executionId),
    onSuccess: (view: ExecutionView) => {
      queryClient.setQueryData(
        queryKeys.worksets.execution(view.workset_id, view.operation_type, view.execution_id),
        view,
      )
    },
  }
}

// A metadata execution's cover deliveries finish after the execution itself,
// so app-lifetime orchestration owns polling; mounted pages share this cache.
export function executionCoversQueryOptions(
  api: ApiClientContract,
  worksetId: string | null | undefined,
  executionId: string | null | undefined,
) {
  return queryOptions({
    queryKey: queryKeys.worksets.covers(worksetId ?? '', executionId ?? ''),
    enabled: Boolean(worksetId && executionId),
    staleTime: 0,
    queryFn: ({ signal }: { signal?: AbortSignal }) =>
      api.getExecutionCovers(worksetId as string, executionId as string, signal),
    retry: false,
  })
}

// Retry or stop the deliveries; the answer is the updated report.
export function changeExecutionCoversMutationOptions(api: ApiClientContract, queryClient: QueryClient) {
  return {
    mutationFn: (input: { worksetId: string, executionId: string, action: 'retry' | 'cancel' }) =>
      api.changeExecutionCovers(input.worksetId, input.executionId, input.action),
    onSuccess: (report: CoverReport, input: { worksetId: string, executionId: string }) => {
      queryClient.setQueryData(queryKeys.worksets.covers(input.worksetId, input.executionId), report)
    },
  }
}

// Execution terminal: refresh everything the outcome can have changed. The
// inventory sync moved entries under the owning library (folder trees read the
// live inventory; list counts change on the next scan), and the operation and
// revision views carry the session refs.
export async function syncAfterExecutionTerminal(
  queryClient: QueryClient,
  worksetId: string,
  operation: OperationType,
): Promise<void> {
  const detail = queryClient.getQueryData<Workset>(queryKeys.worksets.detail(worksetId))
  const libraryId = detail?.library?.library_id ?? queryClient
    .getQueriesData<{ workset: Workset | null }>({
      queryKey: queryKeys.worksets.currentsPrefix(),
    })
    .find(([, record]) => record?.workset?.workset_id === worksetId)?.[1]
    ?.workset
    ?.library
    ?.library_id ?? null
  await Promise.all([
    ...(operation === 'rename' && libraryId
      ? [
          syncRenamedLibrary(queryClient, libraryId, worksetId),
        ]
      : []),
    refreshOrRemoveQueries(queryClient, queryKeys.worksets.listPrefix()),
    refreshOrRemoveQueries(queryClient, queryKeys.worksets.detail(worksetId)),
    refreshOrRemoveQueries(queryClient, queryKeys.worksets.operation(worksetId, operation)),
    refreshOrRemoveQueries(queryClient, queryKeys.worksets.revisionsPrefix(worksetId, operation)),
    // The refreshed detail is the terminal calibration: it carries every
    // component's result (committed/removed/recovery), including any the stream
    // did not get to send.
    refreshOrRemoveQueries(queryClient, queryKeys.worksets.executionsPrefix(worksetId, operation)),
    ...(libraryId
      ? [
          refreshOrRemoveQueries(queryClient, queryKeys.libraries.dirsPrefix(libraryId)),
          refreshOrRemoveQueries(queryClient, queryKeys.libraries.memberTreesPrefix(libraryId)),
        ]
      : []),
  ])
}

// Rename changes the shared batch's members, while their
// frozen plans stay unchanged and must have freshness re-read.
async function syncRenamedLibrary(queryClient: QueryClient, libraryId: string, worksetId: string) {
  await Promise.all([
    refreshOrRemoveQueries(queryClient, queryKeys.worksets.currentPrefix(libraryId)),
    refreshOrRemoveQueries(queryClient, queryKeys.works.all()),
    refreshOrRemoveQueries(queryClient, queryKeys.worksets.detail(worksetId)),
    refreshOrRemoveQueries(queryClient, queryKeys.worksets.operationPrefix(worksetId)),
    refreshOrRemoveQueries(queryClient, queryKeys.worksets.revisionsRecordPrefix(worksetId)),
  ])
}

// A refused start (an eligibility conflict or a stale version) is answered
// with fresh state: the operation and its revisions are re-read so the UI can
// explain against the truth. The request is never retried with a fresh key —
// that would be a second run attempt against the same revision.
export async function syncAfterExecutionRefusal(
  queryClient: QueryClient,
  worksetId: string,
  operation: OperationType,
): Promise<void> {
  await Promise.all([
    refreshOrRemoveQueries(queryClient, queryKeys.worksets.operation(worksetId, operation)),
    refreshOrRemoveQueries(queryClient, queryKeys.worksets.revisionsPrefix(worksetId, operation)),
  ])
}

// Conservative sweep after an unclear transport end: an execution writes to
// disk whenever it ran, so both the workset view and the library inventory may
// have moved.
export async function sweepAfterExecution(queryClient: QueryClient): Promise<void> {
  await Promise.all([
    // Exact session reads belong to recovery; the sweep must not cancel them.
    refreshOrRemoveQueries(queryClient, queryKeys.worksets.all(), {
      predicate: query => query.queryKey[1] !== 'executions' || query.queryKey.length !== 5,
    }),
    refreshOrRemoveQueries(queryClient, queryKeys.libraries.all()),
  ])
}

// Confirmed planning terminal: refresh everything the outcome can have changed.
export async function syncAfterGenerationTerminal(
  queryClient: QueryClient,
  worksetId: string,
  operation: OperationType,
): Promise<void> {
  await Promise.all([
    refreshOrRemoveQueries(queryClient, queryKeys.worksets.listPrefix()),
    refreshOrRemoveQueries(queryClient, queryKeys.worksets.detail(worksetId)),
    refreshOrRemoveQueries(queryClient, queryKeys.worksets.operation(worksetId, operation)),
    refreshOrRemoveQueries(queryClient, queryKeys.worksets.draft(worksetId, operation)),
    refreshOrRemoveQueries(queryClient, queryKeys.worksets.revisionsPrefix(worksetId, operation)),
  ])
}

// Conservative sweep after an unclear transport end: the backend may have
// committed anything. Refreshes every cached workset resource (small domain).
export async function sweepWorksetCaches(queryClient: QueryClient): Promise<void> {
  await refreshOrRemoveQueries(queryClient, queryKeys.worksets.all())
}

// Draft save conflict (409): reload the operation and its draft so the user
// can decide between loading the server version or overwriting it. Local form
// state belongs to the edit session and is intentionally untouched.
export async function syncAfterDraftConflict(
  queryClient: QueryClient,
  worksetId: string,
  operation: OperationType,
): Promise<void> {
  await Promise.all([
    refreshOrRemoveQueries(queryClient, queryKeys.worksets.operation(worksetId, operation)),
    refreshOrRemoveQueries(queryClient, queryKeys.worksets.draft(worksetId, operation)),
  ])
}
