import type { WireSchemas } from '@/lib/api/wire.gen'

/** Complete transport fixtures; overrides stay checked against generated types. */
export function wireGeneration(patch: Partial<WireSchemas['generationViewResponse']> = {}): WireSchemas['generationViewResponse'] {
  return {
    generation_id: 'g',
    workset_id: 'w',
    operation_type: 'conversion',
    status: 'queued',
    total_roots: 0,
    completed_roots: 0,
    current_root: '',
    error_count: 0,
    revision_id: '',
    error_code: '',
    error_message: '',
    started_at: '',
    finished_at: '',
    created_at: '',
    ...patch,
  }
}

export function wireExecution(patch: Partial<WireSchemas['conversion_v1_executionFullViewResponse']> = {}): WireSchemas['conversion_v1_executionFullViewResponse'] {
  return {
    execution_id: 'e',
    workset_id: 'w',
    operation_type: 'conversion',
    plan_id: 'p',
    status: 'queued',
    total_components: 0,
    completed_components: 0,
    total_operations: 0,
    completed_operations: 0,
    current_root: '',
    current_component_id: '',
    current_phase: '',
    components: [],
    error_code: '',
    error_message: '',
    started_at: '',
    finished_at: '',
    created_at: '',
    ...patch,
  }
}

export function wireOperation(patch: Partial<WireSchemas['conversion_v1_operationResponse']> = {}): WireSchemas['conversion_v1_operationResponse'] {
  return {
    workset_id: 'w',
    operation_type: 'conversion',
    version: 1,
    planning_state: 'unplanned',
    current_revision: null,
    active_generation: null,
    latest_generation: null,
    active_execution: null,
    latest_execution: null,
    ...patch,
  }
}

export function wireWorkset(patch: Partial<WireSchemas['worksetResponse']> = {}): WireSchemas['worksetResponse'] {
  return {
    workset_id: 'w',
    title: '',
    version: 1,
    library: null,
    members: [],
    updated_at: '',
    created_at: '',
    ...patch,
  }
}

export function wireDraft(patch: Partial<WireSchemas['conversion_v1_draftResponse']> = {}): WireSchemas['conversion_v1_draftResponse'] {
  return {
    workset_id: 'w',
    operation_type: 'conversion',
    version: 1,
    updated_at: '',
    task: {
      kind: 'conversion',
      schema_version: 1,
      payload: {
        schema_version: 1,
        no_effect_tags: [],
        other_tags: [],
        matched: {},
        unmatched: {},
      },
    },
    ...patch,
  }
}

export function wireRevision(patch: Partial<WireSchemas['conversion_v1_revisionResponse']> = {}): WireSchemas['conversion_v1_revisionResponse'] {
  return {
    plan_id: 'p',
    revision_index: 1,
    created_at: '',
    root_path: '',
    snapshot_token: '',
    status: 'ready',
    summary: {
      operation_count: 0,
      error_count: 0,
      total_count: 0,
      actionable_count: 0,
      summary_reason: '',
    },
    task: {
      kind: 'conversion',
      schema_version: 1,
      payload: {
        policy: {
          schema_version: 1,
          other_tags: [],
          no_effect_tags: [],
          matched: {},
          unmatched: {},
        },
        policy_hash: '',
        classifier: { tags: [], hash: '' },
        summary: {
          component_count: 0,
          blocked_count: 0,
          operation_count: 0,
          error_count: 0,
          summary_reason: '',
        },
      },
    },
    counts: {
      members: 0,
      changed: 0,
      unmet_targets: 0,
      blocked: 0,
      unchanged: 0,
    },
    members: [],
    category_counts: {},
    execution: null,
    ...patch,
  }
}

export function wireJson<K extends keyof WireSchemas>(schema: K, body: WireSchemas[K], headers: Record<string, string> = {}, status = 200): Response {
  void schema
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json', ...headers },
  })
}

export interface WireStreams {
  scan: import('@/lib/api/wire.gen').WireScanEvent
  generation: import('@/lib/api/wire.gen').WireGenerationEvent
  execution: import('@/lib/api/wire.gen').WireExecutionEvent
  media: import('@/lib/api/wire.gen').WireMediaEvent
}

export function wireSSE<F extends keyof WireStreams>(family: F, events: WireStreams[F][]): Response {
  void family
  return new Response(events.map(event => `event: ${event.type}\ndata: ${JSON.stringify(event.data)}\n\n`).join(''), {
    headers: { 'Content-Type': 'text/event-stream' },
  })
}
