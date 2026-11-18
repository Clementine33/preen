// Generated from Go wire schemas. Do not edit; run task contract:generate.

export type ConversionV1ExecutionStartResponse_ExecutionViewResponse =
  | {
      completed_components: number
      completed_operations: number
      components: ConversionV1ExecutionStartResponse_ExecutionComponentResponse[]
      created_at: string
      current_component_id: string
      current_phase: string
      current_root: string
      error_code: string
      error_message: string
      execution_id: string
      finished_at: string
      operation_type: 'conversion'
      options?: Conversion_ExecutionOptions
      plan_id: string
      selected_member_ids?: string[]
      started_at: string
      status: 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'interrupted'
      total_components: number
      total_operations: number
      workset_id: string
    }
  | {
      completed_components: number
      completed_operations: number
      created_at: string
      current_component_id: string
      current_phase: string
      current_root: string
      error_code: string
      error_message: string
      execution_id: string
      finished_at: string
      members: ConversionV1ExecutionStartResponse_ExecutionMembersResponse
      operation_type: 'conversion'
      options?: Conversion_ExecutionOptions
      plan_id: string
      selected_member_ids?: string[]
      started_at: string
      status: 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'interrupted'
      total_components: number
      total_operations: number
      workset_id: string
    }
  | {
      completed_components: number
      completed_operations: number
      components: ConversionV1ExecutionStartResponse_ExecutionComponentResponse[]
      created_at: string
      current_component_id: string
      current_phase: string
      current_root: string
      error_code: string
      error_message: string
      execution_id: string
      finished_at: string
      member: ConversionV1ExecutionStartResponse_ExecutionMemberSummaryResponse
      operation_type: 'conversion'
      options?: Conversion_ExecutionOptions
      plan_id: string
      selected_member_ids?: string[]
      started_at: string
      status: 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'interrupted'
      total_components: number
      total_operations: number
      workset_id: string
    }
export type ConversionV1ExecutionViewResponse =
  | {
      completed_components: number
      completed_operations: number
      components: ConversionV1ExecutionViewResponse_ExecutionComponentResponse[]
      created_at: string
      current_component_id: string
      current_phase: string
      current_root: string
      error_code: string
      error_message: string
      execution_id: string
      finished_at: string
      operation_type: 'conversion'
      options?: Conversion_ExecutionOptions
      plan_id: string
      selected_member_ids?: string[]
      started_at: string
      status: 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'interrupted'
      total_components: number
      total_operations: number
      workset_id: string
    }
  | {
      completed_components: number
      completed_operations: number
      created_at: string
      current_component_id: string
      current_phase: string
      current_root: string
      error_code: string
      error_message: string
      execution_id: string
      finished_at: string
      members: ConversionV1ExecutionViewResponse_ExecutionMembersResponse
      operation_type: 'conversion'
      options?: Conversion_ExecutionOptions
      plan_id: string
      selected_member_ids?: string[]
      started_at: string
      status: 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'interrupted'
      total_components: number
      total_operations: number
      workset_id: string
    }
  | {
      completed_components: number
      completed_operations: number
      components: ConversionV1ExecutionViewResponse_ExecutionComponentResponse[]
      created_at: string
      current_component_id: string
      current_phase: string
      current_root: string
      error_code: string
      error_message: string
      execution_id: string
      finished_at: string
      member: ConversionV1ExecutionViewResponse_ExecutionMemberSummaryResponse
      operation_type: 'conversion'
      options?: Conversion_ExecutionOptions
      plan_id: string
      selected_member_ids?: string[]
      started_at: string
      status: 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'interrupted'
      total_components: number
      total_operations: number
      workset_id: string
    }
export type DraftResponse = ConversionV1DraftResponse | RenameV1DraftResponse | MetadataRefreshV1DraftResponse
export type ErrorWithDetails = ErrorResponse & {
  [k: string]: unknown
}
export type ExecutionComponentResponse =
  | ConversionV1ExecutionComponentResponse
  | RenameV1ExecutionComponentResponse
  | MetadataRefreshV1ExecutionComponentResponse
export type MetadataRefreshV1ExecutionComponentResponse =
  | {
      committed: string[]
      completed_operations: number
      component_id: string
      component_index: number
      error_code?: string
      error_message?: string
      inventory_sync_error?: string
      inventory_synced: boolean
      /**
       * @maxItems 0
       */
      kept_decisions: []
      operations: number
      partition: '' | 'matched' | 'unmatched'
      recovery: string[]
      remaining: string[]
      removed: string[]
      root_path: string
      stage?: 'precheck' | 'materialize' | 'validate' | 'commit' | 'remove' | 'skipped' | 'metadata'
      status: 'pending' | 'succeeded' | 'failed' | 'canceled'
    }
  | {
      committed: string[]
      completed_operations: number
      component_id: string
      component_index: number
      error_code?: string
      error_message?: string
      inventory_sync_error?: string
      inventory_synced: boolean
      /**
       * @maxItems 0
       */
      kept_decisions: []
      operations: number
      partition: '' | 'matched' | 'unmatched'
      recovery: string[]
      remaining: string[]
      removed: string[]
      root_path: string
      stage: 'skipped'
      status: 'pending' | 'succeeded' | 'failed' | 'canceled'
      task_result: Refresh_Outcome
    }
  | {
      committed: string[]
      completed_operations: number
      component_id: string
      component_index: number
      error_code?: string
      error_message?: string
      inventory_sync_error?: string
      inventory_synced: boolean
      /**
       * @maxItems 0
       */
      kept_decisions: []
      operations: number
      partition: '' | 'matched' | 'unmatched'
      recovery: string[]
      remaining: string[]
      removed: string[]
      root_path: string
      stage: 'metadata'
      status: 'pending' | 'succeeded' | 'failed' | 'canceled'
      task_result: Refresh_WorkResult
    }
export type ExecutionFullViewResponse =
  ConversionV1ExecutionFullViewResponse | RenameV1ExecutionFullViewResponse | MetadataRefreshV1ExecutionFullViewResponse
export type MetadataRefreshV1ExecutionFullViewResponse_ExecutionComponentResponse =
  | {
      committed: string[]
      completed_operations: number
      component_id: string
      component_index: number
      error_code?: string
      error_message?: string
      inventory_sync_error?: string
      inventory_synced: boolean
      /**
       * @maxItems 0
       */
      kept_decisions: []
      operations: number
      partition: '' | 'matched' | 'unmatched'
      recovery: string[]
      remaining: string[]
      removed: string[]
      root_path: string
      stage?: 'precheck' | 'materialize' | 'validate' | 'commit' | 'remove' | 'skipped' | 'metadata'
      status: 'pending' | 'succeeded' | 'failed' | 'canceled'
    }
  | {
      committed: string[]
      completed_operations: number
      component_id: string
      component_index: number
      error_code?: string
      error_message?: string
      inventory_sync_error?: string
      inventory_synced: boolean
      /**
       * @maxItems 0
       */
      kept_decisions: []
      operations: number
      partition: '' | 'matched' | 'unmatched'
      recovery: string[]
      remaining: string[]
      removed: string[]
      root_path: string
      stage: 'skipped'
      status: 'pending' | 'succeeded' | 'failed' | 'canceled'
      task_result: Refresh_Outcome
    }
  | {
      committed: string[]
      completed_operations: number
      component_id: string
      component_index: number
      error_code?: string
      error_message?: string
      inventory_sync_error?: string
      inventory_synced: boolean
      /**
       * @maxItems 0
       */
      kept_decisions: []
      operations: number
      partition: '' | 'matched' | 'unmatched'
      recovery: string[]
      remaining: string[]
      removed: string[]
      root_path: string
      stage: 'metadata'
      status: 'pending' | 'succeeded' | 'failed' | 'canceled'
      task_result: Refresh_WorkResult
    }
export type ExecutionMemberViewResponse =
  | ConversionV1ExecutionMemberViewResponse
  | RenameV1ExecutionMemberViewResponse
  | MetadataRefreshV1ExecutionMemberViewResponse
export type MetadataRefreshV1ExecutionMemberViewResponse_ExecutionComponentResponse =
  | {
      committed: string[]
      completed_operations: number
      component_id: string
      component_index: number
      error_code?: string
      error_message?: string
      inventory_sync_error?: string
      inventory_synced: boolean
      /**
       * @maxItems 0
       */
      kept_decisions: []
      operations: number
      partition: '' | 'matched' | 'unmatched'
      recovery: string[]
      remaining: string[]
      removed: string[]
      root_path: string
      stage?: 'precheck' | 'materialize' | 'validate' | 'commit' | 'remove' | 'skipped' | 'metadata'
      status: 'pending' | 'succeeded' | 'failed' | 'canceled'
    }
  | {
      committed: string[]
      completed_operations: number
      component_id: string
      component_index: number
      error_code?: string
      error_message?: string
      inventory_sync_error?: string
      inventory_synced: boolean
      /**
       * @maxItems 0
       */
      kept_decisions: []
      operations: number
      partition: '' | 'matched' | 'unmatched'
      recovery: string[]
      remaining: string[]
      removed: string[]
      root_path: string
      stage: 'skipped'
      status: 'pending' | 'succeeded' | 'failed' | 'canceled'
      task_result: Refresh_Outcome
    }
  | {
      committed: string[]
      completed_operations: number
      component_id: string
      component_index: number
      error_code?: string
      error_message?: string
      inventory_sync_error?: string
      inventory_synced: boolean
      /**
       * @maxItems 0
       */
      kept_decisions: []
      operations: number
      partition: '' | 'matched' | 'unmatched'
      recovery: string[]
      remaining: string[]
      removed: string[]
      root_path: string
      stage: 'metadata'
      status: 'pending' | 'succeeded' | 'failed' | 'canceled'
      task_result: Refresh_WorkResult
    }
export type ExecutionMembersViewResponse =
  | ConversionV1ExecutionMembersViewResponse
  | RenameV1ExecutionMembersViewResponse
  | MetadataRefreshV1ExecutionMembersViewResponse
export type ExecutionProgressResponse =
  ConversionV1ExecutionProgressResponse | RenameV1ExecutionProgressResponse | MetadataRefreshV1ExecutionProgressResponse
export type ExecutionStartResponse =
  ConversionV1ExecutionStartResponse | RenameV1ExecutionStartResponse | MetadataRefreshV1ExecutionStartResponse
export type RenameV1ExecutionStartResponse_ExecutionViewResponse =
  | {
      completed_components: number
      completed_operations: number
      components: RenameV1ExecutionStartResponse_ExecutionComponentResponse[]
      created_at: string
      current_component_id: string
      current_phase: string
      current_root: string
      error_code: string
      error_message: string
      execution_id: string
      finished_at: string
      operation_type: 'rename'
      plan_id: string
      selected_member_ids?: string[]
      started_at: string
      status: 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'interrupted'
      total_components: number
      total_operations: number
      workset_id: string
    }
  | {
      completed_components: number
      completed_operations: number
      created_at: string
      current_component_id: string
      current_phase: string
      current_root: string
      error_code: string
      error_message: string
      execution_id: string
      finished_at: string
      members: RenameV1ExecutionStartResponse_ExecutionMembersResponse
      operation_type: 'rename'
      plan_id: string
      selected_member_ids?: string[]
      started_at: string
      status: 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'interrupted'
      total_components: number
      total_operations: number
      workset_id: string
    }
  | {
      completed_components: number
      completed_operations: number
      components: RenameV1ExecutionStartResponse_ExecutionComponentResponse[]
      created_at: string
      current_component_id: string
      current_phase: string
      current_root: string
      error_code: string
      error_message: string
      execution_id: string
      finished_at: string
      member: RenameV1ExecutionStartResponse_ExecutionMemberSummaryResponse
      operation_type: 'rename'
      plan_id: string
      selected_member_ids?: string[]
      started_at: string
      status: 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'interrupted'
      total_components: number
      total_operations: number
      workset_id: string
    }
export type MetadataRefreshV1ExecutionStartResponse_ExecutionViewResponse =
  | {
      completed_components: number
      completed_operations: number
      components: MetadataRefreshV1ExecutionStartResponse_ExecutionComponentResponse[]
      created_at: string
      current_component_id: string
      current_phase: string
      current_root: string
      error_code: string
      error_message: string
      execution_id: string
      finished_at: string
      operation_type: 'metadata_refresh'
      options?: Refresh_Common
      plan_id: string
      selected_member_ids?: string[]
      started_at: string
      status: 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'interrupted'
      total_components: number
      total_operations: number
      workset_id: string
    }
  | {
      completed_components: number
      completed_operations: number
      created_at: string
      current_component_id: string
      current_phase: string
      current_root: string
      error_code: string
      error_message: string
      execution_id: string
      finished_at: string
      members: MetadataRefreshV1ExecutionStartResponse_ExecutionMembersResponse
      operation_type: 'metadata_refresh'
      options?: Refresh_Common
      plan_id: string
      selected_member_ids?: string[]
      started_at: string
      status: 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'interrupted'
      total_components: number
      total_operations: number
      workset_id: string
    }
  | {
      completed_components: number
      completed_operations: number
      components: MetadataRefreshV1ExecutionStartResponse_ExecutionComponentResponse[]
      created_at: string
      current_component_id: string
      current_phase: string
      current_root: string
      error_code: string
      error_message: string
      execution_id: string
      finished_at: string
      member: MetadataRefreshV1ExecutionStartResponse_ExecutionMemberSummaryResponse
      operation_type: 'metadata_refresh'
      options?: Refresh_Common
      plan_id: string
      selected_member_ids?: string[]
      started_at: string
      status: 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'interrupted'
      total_components: number
      total_operations: number
      workset_id: string
    }
export type MetadataRefreshV1ExecutionStartResponse_ExecutionComponentResponse =
  | {
      committed: string[]
      completed_operations: number
      component_id: string
      component_index: number
      error_code?: string
      error_message?: string
      inventory_sync_error?: string
      inventory_synced: boolean
      /**
       * @maxItems 0
       */
      kept_decisions: []
      operations: number
      partition: '' | 'matched' | 'unmatched'
      recovery: string[]
      remaining: string[]
      removed: string[]
      root_path: string
      stage?: 'precheck' | 'materialize' | 'validate' | 'commit' | 'remove' | 'skipped' | 'metadata'
      status: 'pending' | 'succeeded' | 'failed' | 'canceled'
    }
  | {
      committed: string[]
      completed_operations: number
      component_id: string
      component_index: number
      error_code?: string
      error_message?: string
      inventory_sync_error?: string
      inventory_synced: boolean
      /**
       * @maxItems 0
       */
      kept_decisions: []
      operations: number
      partition: '' | 'matched' | 'unmatched'
      recovery: string[]
      remaining: string[]
      removed: string[]
      root_path: string
      stage: 'skipped'
      status: 'pending' | 'succeeded' | 'failed' | 'canceled'
      task_result: Refresh_Outcome
    }
  | {
      committed: string[]
      completed_operations: number
      component_id: string
      component_index: number
      error_code?: string
      error_message?: string
      inventory_sync_error?: string
      inventory_synced: boolean
      /**
       * @maxItems 0
       */
      kept_decisions: []
      operations: number
      partition: '' | 'matched' | 'unmatched'
      recovery: string[]
      remaining: string[]
      removed: string[]
      root_path: string
      stage: 'metadata'
      status: 'pending' | 'succeeded' | 'failed' | 'canceled'
      task_result: Refresh_WorkResult
    }
export type ExecutionViewResponse =
  ConversionV1ExecutionViewResponse | RenameV1ExecutionViewResponse | MetadataRefreshV1ExecutionViewResponse
export type RenameV1ExecutionViewResponse =
  | {
      completed_components: number
      completed_operations: number
      components: RenameV1ExecutionViewResponse_ExecutionComponentResponse[]
      created_at: string
      current_component_id: string
      current_phase: string
      current_root: string
      error_code: string
      error_message: string
      execution_id: string
      finished_at: string
      operation_type: 'rename'
      plan_id: string
      selected_member_ids?: string[]
      started_at: string
      status: 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'interrupted'
      total_components: number
      total_operations: number
      workset_id: string
    }
  | {
      completed_components: number
      completed_operations: number
      created_at: string
      current_component_id: string
      current_phase: string
      current_root: string
      error_code: string
      error_message: string
      execution_id: string
      finished_at: string
      members: RenameV1ExecutionViewResponse_ExecutionMembersResponse
      operation_type: 'rename'
      plan_id: string
      selected_member_ids?: string[]
      started_at: string
      status: 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'interrupted'
      total_components: number
      total_operations: number
      workset_id: string
    }
  | {
      completed_components: number
      completed_operations: number
      components: RenameV1ExecutionViewResponse_ExecutionComponentResponse[]
      created_at: string
      current_component_id: string
      current_phase: string
      current_root: string
      error_code: string
      error_message: string
      execution_id: string
      finished_at: string
      member: RenameV1ExecutionViewResponse_ExecutionMemberSummaryResponse
      operation_type: 'rename'
      plan_id: string
      selected_member_ids?: string[]
      started_at: string
      status: 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'interrupted'
      total_components: number
      total_operations: number
      workset_id: string
    }
export type MetadataRefreshV1ExecutionViewResponse =
  | {
      completed_components: number
      completed_operations: number
      components: MetadataRefreshV1ExecutionViewResponse_ExecutionComponentResponse[]
      created_at: string
      current_component_id: string
      current_phase: string
      current_root: string
      error_code: string
      error_message: string
      execution_id: string
      finished_at: string
      operation_type: 'metadata_refresh'
      options?: Refresh_Common
      plan_id: string
      selected_member_ids?: string[]
      started_at: string
      status: 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'interrupted'
      total_components: number
      total_operations: number
      workset_id: string
    }
  | {
      completed_components: number
      completed_operations: number
      created_at: string
      current_component_id: string
      current_phase: string
      current_root: string
      error_code: string
      error_message: string
      execution_id: string
      finished_at: string
      members: MetadataRefreshV1ExecutionViewResponse_ExecutionMembersResponse
      operation_type: 'metadata_refresh'
      options?: Refresh_Common
      plan_id: string
      selected_member_ids?: string[]
      started_at: string
      status: 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'interrupted'
      total_components: number
      total_operations: number
      workset_id: string
    }
  | {
      completed_components: number
      completed_operations: number
      components: MetadataRefreshV1ExecutionViewResponse_ExecutionComponentResponse[]
      created_at: string
      current_component_id: string
      current_phase: string
      current_root: string
      error_code: string
      error_message: string
      execution_id: string
      finished_at: string
      member: MetadataRefreshV1ExecutionViewResponse_ExecutionMemberSummaryResponse
      operation_type: 'metadata_refresh'
      options?: Refresh_Common
      plan_id: string
      selected_member_ids?: string[]
      started_at: string
      status: 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'interrupted'
      total_components: number
      total_operations: number
      workset_id: string
    }
export type MetadataRefreshV1ExecutionViewResponse_ExecutionComponentResponse =
  | {
      committed: string[]
      completed_operations: number
      component_id: string
      component_index: number
      error_code?: string
      error_message?: string
      inventory_sync_error?: string
      inventory_synced: boolean
      /**
       * @maxItems 0
       */
      kept_decisions: []
      operations: number
      partition: '' | 'matched' | 'unmatched'
      recovery: string[]
      remaining: string[]
      removed: string[]
      root_path: string
      stage?: 'precheck' | 'materialize' | 'validate' | 'commit' | 'remove' | 'skipped' | 'metadata'
      status: 'pending' | 'succeeded' | 'failed' | 'canceled'
    }
  | {
      committed: string[]
      completed_operations: number
      component_id: string
      component_index: number
      error_code?: string
      error_message?: string
      inventory_sync_error?: string
      inventory_synced: boolean
      /**
       * @maxItems 0
       */
      kept_decisions: []
      operations: number
      partition: '' | 'matched' | 'unmatched'
      recovery: string[]
      remaining: string[]
      removed: string[]
      root_path: string
      stage: 'skipped'
      status: 'pending' | 'succeeded' | 'failed' | 'canceled'
      task_result: Refresh_Outcome
    }
  | {
      committed: string[]
      completed_operations: number
      component_id: string
      component_index: number
      error_code?: string
      error_message?: string
      inventory_sync_error?: string
      inventory_synced: boolean
      /**
       * @maxItems 0
       */
      kept_decisions: []
      operations: number
      partition: '' | 'matched' | 'unmatched'
      recovery: string[]
      remaining: string[]
      removed: string[]
      root_path: string
      stage: 'metadata'
      status: 'pending' | 'succeeded' | 'failed' | 'canceled'
      task_result: Refresh_WorkResult
    }
export type MemberConclusionResponse =
  ConversionV1MemberConclusionResponse | RenameV1MemberConclusionResponse | MetadataRefreshV1MemberConclusionResponse
export type MemberReviewResponse =
  ConversionV1MemberReviewResponse | RenameV1MemberReviewResponse | MetadataRefreshV1MemberReviewResponse
export type OperationResponse =
  ConversionV1OperationResponse | RenameV1OperationResponse | MetadataRefreshV1OperationResponse
export type RevisionMemberResponse =
  ConversionV1RevisionMemberResponse | RenameV1RevisionMemberResponse | MetadataRefreshV1RevisionMemberResponse
export type RevisionResponse =
  ConversionV1RevisionResponse | RenameV1RevisionResponse | MetadataRefreshV1RevisionResponse
export type TaskDraftEnvelope =
  ConversionV1TaskDraftEnvelope | RenameV1TaskDraftEnvelope | MetadataRefreshV1TaskDraftEnvelope
export type TaskEnvelopeResponse =
  ConversionV1TaskEnvelopeResponse | RenameV1TaskEnvelopeResponse | MetadataRefreshV1TaskEnvelopeResponse
export type TaskPayloadResponse =
  ConversionV1TaskPayloadResponse | RenameV1TaskPayloadResponse | MetadataRefreshV1TaskPayloadResponse
export type TaskSettingsRequest =
  ConversionV1TaskSettingsRequest | RenameV1TaskSettingsRequest | MetadataRefreshV1TaskSettingsRequest
export type TaskSettingsResponse =
  ConversionV1TaskSettingsResponse | RenameV1TaskSettingsResponse | MetadataRefreshV1TaskSettingsResponse
export type WorkContentCopyResponse_WorkContentNodeResponse = WorkContentDirNodeJSON | WorkContentFileNodeJSON
export type WorkContentDirNodeJSON_WorkContentNodeResponse = WorkContentDirNodeJSON1 | WorkContentFileNodeJSON
export type WorkContentNodeResponse = WorkContentDirNodeJSON2 | WorkContentFileNodeJSON
export type WorkContentResponse_WorkContentNodeResponse = WorkContentDirNodeJSON2 | WorkContentFileNodeJSON

export interface WireSchemas {
  appliedGroupResponse: AppliedGroupResponse
  audioOutputSpecDTO: AudioOutputSpecDTO
  audioQualityDTO: AudioQualityDTO
  circleFacetsResponse: CircleFacetsResponse
  classifierCustomTagItem: ClassifierCustomTagItem
  classifierTagCreateRequest: ClassifierTagCreateRequest
  classifierTagLibraryResponse: ClassifierTagLibraryResponse
  cleanupCategoriesRequest: CleanupCategoriesRequest
  cleanupCountResponse: CleanupCountResponse
  cleanupPreviewResponse: CleanupPreviewResponse
  cleanupRuleResponse: CleanupRuleResponse
  cleanupRunEnvelopeResponse: CleanupRunEnvelopeResponse
  cleanupRunResponse: CleanupRunResponse
  collectionDetailResponse: CollectionDetailResponse
  collectionListResponse: CollectionListResponse
  collectionNameRequest: CollectionNameRequest
  collectionResponse: CollectionResponse
  collectionWorkMoveRequest: CollectionWorkMoveRequest
  collectionWorksAppendRequest: CollectionWorksAppendRequest
  collectionWorksAppendResponse: CollectionWorksAppendResponse
  conversion_CommonSettings: Conversion_CommonSettings
  conversion_CommonSettings_request: Conversion_CommonSettingsRequest
  conversion_DraftDoc: Conversion_DraftDoc
  conversion_DraftDoc_request: Conversion_DraftDocRequest
  conversion_ExecutionOptions: Conversion_ExecutionOptions
  conversion_FrozenMemberConclusion: Conversion_FrozenMemberConclusion
  conversion_v1_draftResponse: ConversionV1DraftResponse
  conversion_v1_executionComponentResponse: ConversionV1ExecutionComponentResponse
  conversion_v1_executionFullViewResponse: ConversionV1ExecutionFullViewResponse
  conversion_v1_executionMemberViewResponse: ConversionV1ExecutionMemberViewResponse
  conversion_v1_executionMembersViewResponse: ConversionV1ExecutionMembersViewResponse
  conversion_v1_executionProgressResponse: ConversionV1ExecutionProgressResponse
  conversion_v1_executionStartResponse: ConversionV1ExecutionStartResponse
  conversion_v1_executionViewResponse: ConversionV1ExecutionViewResponse
  conversion_v1_memberConclusionResponse: ConversionV1MemberConclusionResponse
  conversion_v1_memberReviewResponse: ConversionV1MemberReviewResponse
  conversion_v1_operationResponse: ConversionV1OperationResponse
  conversion_v1_revisionMemberResponse: ConversionV1RevisionMemberResponse
  conversion_v1_revisionResponse: ConversionV1RevisionResponse
  conversion_v1_taskDraftEnvelope: ConversionV1TaskDraftEnvelope
  conversion_v1_taskEnvelopeResponse: ConversionV1TaskEnvelopeResponse
  conversion_v1_taskPayloadResponse: ConversionV1TaskPayloadResponse
  conversion_v1_taskSettingsRequest: ConversionV1TaskSettingsRequest
  conversion_v1_taskSettingsResponse: ConversionV1TaskSettingsResponse
  copyResponse: CopyResponse
  coverDestinationResponse: CoverDestinationResponse
  coverReportResponse: CoverReportResponse
  coverTargetResponse: CoverTargetResponse
  currentRecordResponse: CurrentRecordResponse
  currentRevisionResponse: CurrentRevisionResponse
  desiredProfileDTO: DesiredProfileDTO
  dirListResponse: DirListResponse
  dirResponse: DirResponse
  dirrename_Common: Dirrename_Common
  dirrename_Common_request: Dirrename_CommonRequest
  dirrename_Draft: Dirrename_Draft
  dirrename_Draft_request: Dirrename_DraftRequest
  dirrename_Outcome: Dirrename_Outcome
  dirrename_Result: Dirrename_Result
  draftDocumentRequest: DraftDocumentRequest
  draftMemberRequest: DraftMemberRequest
  draftOverridesRequest: DraftOverridesRequest
  draftResponse: DraftResponse
  editionResponse: EditionResponse
  editionSlotResponse: EditionSlotResponse
  emptyEvent: EmptyEvent
  emptyRequest: EmptyRequest
  errorResponse: ErrorResponse
  errorWithDetails: ErrorWithDetails
  executionComponentResponse: ExecutionComponentResponse
  executionFailedEvent: ExecutionFailedEvent
  executionFullViewResponse: ExecutionFullViewResponse
  executionMemberCountsResponse: ExecutionMemberCountsResponse
  executionMemberEvent: ExecutionMemberEvent
  executionMemberSummaryResponse: ExecutionMemberSummaryResponse
  executionMemberViewResponse: ExecutionMemberViewResponse
  executionMembersResponse: ExecutionMembersResponse
  executionMembersViewResponse: ExecutionMembersViewResponse
  executionProgressEvent: ExecutionProgressEvent
  executionProgressResponse: ExecutionProgressResponse
  executionRefResponse: ExecutionRefResponse
  executionStartRequest: ExecutionStartRequest
  executionStartResponse: ExecutionStartResponse
  executionStoppedEvent: ExecutionStoppedEvent
  executionSucceededEvent: ExecutionSucceededEvent
  executionViewResponse: ExecutionViewResponse
  facetResponse: FacetResponse
  fileOperationItem: FileOperationItem
  fileOperationItemResponse: FileOperationItemResponse
  fileOperationRefreshResponse: FileOperationRefreshResponse
  fileOperationRequest: FileOperationRequest
  fileOperationResponse: FileOperationResponse
  generationCompletedEvent: GenerationCompletedEvent
  generationFailedEvent: GenerationFailedEvent
  generationProgressEvent: GenerationProgressEvent
  generationProgressResponse: GenerationProgressResponse
  generationReplayRevisionResponse: GenerationReplayRevisionResponse
  generationStartResponse: GenerationStartResponse
  generationStoppedEvent: GenerationStoppedEvent
  generationSummaryResponse: GenerationSummaryResponse
  generationViewResponse: GenerationViewResponse
  healthResponse: HealthResponse
  ignoredGroupResponse: IgnoredGroupResponse
  importResponse: ImportResponse
  libraryCreateRequest: LibraryCreateRequest
  libraryListResponse: LibraryListResponse
  libraryPatchRequest: LibraryPatchRequest
  libraryRefResponse: LibraryRefResponse
  libraryResponse: LibraryResponse
  listeningResponse: ListeningResponse
  listeningSummaryResponse: ListeningSummaryResponse
  localCoverResponse: LocalCoverResponse
  mediaFactsLibraryStat: MediaFactsLibraryStat
  mediaFactsStatus: MediaFactsStatus
  memberConclusionResponse: MemberConclusionResponse
  memberResponse: MemberResponse
  memberReviewResponse: MemberReviewResponse
  memberTreeNodeResponse: MemberTreeNodeResponse
  metadata_refresh_v1_draftResponse: MetadataRefreshV1DraftResponse
  metadata_refresh_v1_executionComponentResponse: MetadataRefreshV1ExecutionComponentResponse
  metadata_refresh_v1_executionFullViewResponse: MetadataRefreshV1ExecutionFullViewResponse
  metadata_refresh_v1_executionMemberViewResponse: MetadataRefreshV1ExecutionMemberViewResponse
  metadata_refresh_v1_executionMembersViewResponse: MetadataRefreshV1ExecutionMembersViewResponse
  metadata_refresh_v1_executionProgressResponse: MetadataRefreshV1ExecutionProgressResponse
  metadata_refresh_v1_executionStartResponse: MetadataRefreshV1ExecutionStartResponse
  metadata_refresh_v1_executionViewResponse: MetadataRefreshV1ExecutionViewResponse
  metadata_refresh_v1_memberConclusionResponse: MetadataRefreshV1MemberConclusionResponse
  metadata_refresh_v1_memberReviewResponse: MetadataRefreshV1MemberReviewResponse
  metadata_refresh_v1_operationResponse: MetadataRefreshV1OperationResponse
  metadata_refresh_v1_revisionMemberResponse: MetadataRefreshV1RevisionMemberResponse
  metadata_refresh_v1_revisionResponse: MetadataRefreshV1RevisionResponse
  metadata_refresh_v1_taskDraftEnvelope: MetadataRefreshV1TaskDraftEnvelope
  metadata_refresh_v1_taskEnvelopeResponse: MetadataRefreshV1TaskEnvelopeResponse
  metadata_refresh_v1_taskPayloadResponse: MetadataRefreshV1TaskPayloadResponse
  metadata_refresh_v1_taskSettingsRequest: MetadataRefreshV1TaskSettingsRequest
  metadata_refresh_v1_taskSettingsResponse: MetadataRefreshV1TaskSettingsResponse
  operationResponse: OperationResponse
  outboundProxyPutRequest: OutboundProxyPutRequest
  outboundProxyResponse: OutboundProxyResponse
  planSummaryResponse: PlanSummaryResponse
  preferencesResponse: PreferencesResponse
  preferencesUpdateRequest: PreferencesUpdateRequest
  randomWorkResponse: RandomWorkResponse
  reconcile_ComponentOutcome: Reconcile_ComponentOutcome
  reconcile_FileDecision: Reconcile_FileDecision
  reconcile_Policy: Reconcile_Policy
  reconcile_StepSummary: Reconcile_StepSummary
  reconcile_VariantDecision: Reconcile_VariantDecision
  recordCreateRequest: RecordCreateRequest
  recordCreateResponse: RecordCreateResponse
  refresh_Common: Refresh_Common
  refresh_Common_request: Refresh_CommonRequest
  refresh_Draft: Refresh_Draft
  refresh_Draft_request: Refresh_DraftRequest
  refresh_Outcome: Refresh_Outcome
  refresh_PlanSummary: Refresh_PlanSummary
  refresh_WorkResult: Refresh_WorkResult
  relationResponse: RelationResponse
  relationWarningResponse: RelationWarningResponse
  rename_v1_draftResponse: RenameV1DraftResponse
  rename_v1_executionComponentResponse: RenameV1ExecutionComponentResponse
  rename_v1_executionFullViewResponse: RenameV1ExecutionFullViewResponse
  rename_v1_executionMemberViewResponse: RenameV1ExecutionMemberViewResponse
  rename_v1_executionMembersViewResponse: RenameV1ExecutionMembersViewResponse
  rename_v1_executionProgressResponse: RenameV1ExecutionProgressResponse
  rename_v1_executionStartResponse: RenameV1ExecutionStartResponse
  rename_v1_executionViewResponse: RenameV1ExecutionViewResponse
  rename_v1_memberConclusionResponse: RenameV1MemberConclusionResponse
  rename_v1_memberReviewResponse: RenameV1MemberReviewResponse
  rename_v1_operationResponse: RenameV1OperationResponse
  rename_v1_revisionMemberResponse: RenameV1RevisionMemberResponse
  rename_v1_revisionResponse: RenameV1RevisionResponse
  rename_v1_taskDraftEnvelope: RenameV1TaskDraftEnvelope
  rename_v1_taskEnvelopeResponse: RenameV1TaskEnvelopeResponse
  rename_v1_taskPayloadResponse: RenameV1TaskPayloadResponse
  rename_v1_taskSettingsRequest: RenameV1TaskSettingsRequest
  rename_v1_taskSettingsResponse: RenameV1TaskSettingsResponse
  revisionCountsResponse: RevisionCountsResponse
  revisionMemberResponse: RevisionMemberResponse
  revisionResponse: RevisionResponse
  rootValidationResponse: RootValidationResponse
  scanEventData: ScanEventData
  scanRequest: ScanRequest
  selectionRequest: SelectionRequest
  sessionCreateRequest: SessionCreateRequest
  sessionStatusEvent: SessionStatusEvent
  skippedFolderResponse: SkippedFolderResponse
  sourceEntityResponse: SourceEntityResponse
  sseError: ErrorResponse
  tagFacetsResponse: TagFacetsResponse
  taskClassifierSnapshot: TaskClassifierSnapshot
  taskDraftEnvelope: TaskDraftEnvelope
  taskEnvelopeResponse: TaskEnvelopeResponse
  taskPayloadResponse: TaskPayloadResponse
  taskSettingsRequest: TaskSettingsRequest
  taskSettingsResponse: TaskSettingsResponse
  treeNodeResponse: TreeNodeResponse
  treeResponse: TreeResponse
  updateCheckResponse: UpdateCheckResponse
  userDataResponse: UserDataResponse
  userTagListResponse: UserTagListResponse
  userTagRequest: UserTagRequest
  userTagResponse: UserTagResponse
  voiceActorFacetsResponse: VoiceActorFacetsResponse
  warningResponse: WarningResponse
  workContentCopyResponse: WorkContentCopyResponse
  workContentDirNodeJSON: WorkContentDirNodeJSON2
  workContentFileNodeJSON: WorkContentFileNodeJSON
  workContentNodeResponse: WorkContentNodeResponse
  workContentResponse: WorkContentResponse
  workDetailResponse: WorkDetailResponse
  workImportCircleRequest: WorkImportCircleRequest
  workImportCoreRequest: WorkImportCoreRequest
  workImportEditionItemRequest: WorkImportEditionItemRequest
  workImportEditionsRequest: WorkImportEditionsRequest
  workImportRelationsRequest: WorkImportRelationsRequest
  workImportStatisticsRequest: WorkImportStatisticsRequest
  workImportTagRequest: WorkImportTagRequest
  workImportVoiceActorRequest: WorkImportVoiceActorRequest
  workListResponse: WorkListResponse
  workListeningRequest: WorkListeningRequest
  workMetadataImportRequest: WorkMetadataImportRequest
  workSummaryResponse: WorkSummaryResponse
  workUserDataPatchRequest: WorkUserDataPatchRequest
  workUserTagsRequest: WorkUserTagsRequest
  worksetListResponse: WorksetListResponse
  worksetPatchRequest: WorksetPatchRequest
  worksetResponse: WorksetResponse
  workset_PlanSummary: Workset_PlanSummary
}
export interface AppliedGroupResponse {
  data_group: string
  dl_count?: number
}
export interface AudioOutputSpecDTO {
  codec: string
  quality?: AudioOutputSpecDTO_AudioQualityDTO
}
export interface AudioOutputSpecDTO_AudioQualityDTO {
  bitrate?: number
  kind: string
}
export interface AudioQualityDTO {
  bitrate?: number
  kind: string
}
export interface CircleFacetsResponse {
  circles: CircleFacetsResponse_FacetResponse[]
}
export interface CircleFacetsResponse_FacetResponse {
  id: string
  name: string
  work_count: number
}
export interface ClassifierCustomTagItem {
  created_at?: string
  id: number
  tag: string
}
export interface ClassifierTagCreateRequest {
  tag?: string
}
export interface ClassifierTagLibraryResponse {
  custom_tags: ClassifierTagLibraryResponse_ClassifierCustomTagItem[]
  default_no_effect_tags: string[]
  default_other_tags: string[]
}
export interface ClassifierTagLibraryResponse_ClassifierCustomTagItem {
  created_at?: string
  id: number
  tag: string
}
export interface CleanupCategoriesRequest {
  categories?: (
    | 'scan_sessions'
    | 'plan_generations'
    | 'generation_records'
    | 'works'
    | 'relation_reports'
    | 'source_tags'
    | 'source_voice_actors'
    | 'source_circles'
  )[]
}
export interface CleanupCountResponse {
  checked: number
  deleted: number
  skipped: number
}
export interface CleanupPreviewResponse {
  candidates: {
    [k: string]: number
  } | null
  categories: (
    | 'scan_sessions'
    | 'plan_generations'
    | 'generation_records'
    | 'works'
    | 'relation_reports'
    | 'source_tags'
    | 'source_voice_actors'
    | 'source_circles'
  )[]
  checked_at: string
  rules: {
    [k: string]: CleanupPreviewResponse_CleanupRuleResponse
  } | null
}
export interface CleanupPreviewResponse_CleanupRuleResponse {
  protection: string
}
export interface CleanupRuleResponse {
  protection: string
}
export interface CleanupRunEnvelopeResponse {
  run: CleanupRunEnvelopeResponse_CleanupRunResponse | null
}
export interface CleanupRunEnvelopeResponse_CleanupRunResponse {
  categories: (
    | 'scan_sessions'
    | 'plan_generations'
    | 'generation_records'
    | 'works'
    | 'relation_reports'
    | 'source_tags'
    | 'source_voice_actors'
    | 'source_circles'
  )[]
  counts: {
    [k: string]: CleanupRunEnvelopeResponse_CleanupCountResponse
  } | null
  finished_at: string | null
  phase:
    | 'scan_sessions'
    | 'plan_generations'
    | 'generation_records'
    | 'works'
    | 'relation_reports'
    | 'source_tags'
    | 'source_voice_actors'
    | 'source_circles'
  reason: string
  run_id: string
  started_at: string
  status: 'running' | 'stopping' | 'stopped' | 'succeeded' | 'failed' | 'interrupted'
}
export interface CleanupRunEnvelopeResponse_CleanupCountResponse {
  checked: number
  deleted: number
  skipped: number
}
export interface CleanupRunResponse {
  categories: (
    | 'scan_sessions'
    | 'plan_generations'
    | 'generation_records'
    | 'works'
    | 'relation_reports'
    | 'source_tags'
    | 'source_voice_actors'
    | 'source_circles'
  )[]
  counts: {
    [k: string]: CleanupRunResponse_CleanupCountResponse
  } | null
  finished_at: string | null
  phase:
    | 'scan_sessions'
    | 'plan_generations'
    | 'generation_records'
    | 'works'
    | 'relation_reports'
    | 'source_tags'
    | 'source_voice_actors'
    | 'source_circles'
  reason: string
  run_id: string
  started_at: string
  status: 'running' | 'stopping' | 'stopped' | 'succeeded' | 'failed' | 'interrupted'
}
export interface CleanupRunResponse_CleanupCountResponse {
  checked: number
  deleted: number
  skipped: number
}
export interface CollectionDetailResponse {
  created_at: string
  id: string
  name: string
  total: number
  updated_at: string
  work_count: number
  works: CollectionDetailResponse_WorkSummaryResponse[]
}
export interface CollectionDetailResponse_WorkSummaryResponse {
  age_category: number | null
  circle: string | null
  collected_at: string
  copy_count: number
  cover_url: string | null
  dl_count: number | null
  favorite: boolean
  has_note: boolean
  language: string | null
  listening: CollectionDetailResponse_ListeningSummaryResponse
  local_cover: CollectionDetailResponse_LocalCoverResponse | null
  nsfw: boolean | null
  rating: number | null
  release_date: string | null
  rj: string
  title: string | null
  user_tags: CollectionDetailResponse_UserTagResponse[]
}
export interface CollectionDetailResponse_ListeningSummaryResponse {
  state: 'none' | 'listening' | 'finished'
  track_count: number | null
  track_index: number | null
  updated_at: string | null
}
export interface CollectionDetailResponse_LocalCoverResponse {
  copy_id: string
  rel_path: string
  thumb_v: string
  version: string
}
export interface CollectionDetailResponse_UserTagResponse {
  id: string
  name: string
}
export interface CollectionListResponse {
  collections: CollectionListResponse_CollectionResponse[]
  total: number
}
export interface CollectionListResponse_CollectionResponse {
  created_at: string
  id: string
  name: string
  updated_at: string
  work_count: number
}
export interface CollectionNameRequest {
  name?: string
}
export interface CollectionResponse {
  created_at: string
  id: string
  name: string
  updated_at: string
  work_count: number
}
export interface CollectionWorkMoveRequest {
  before_rj?: string | null
}
export interface CollectionWorksAppendRequest {
  rjs?: string[]
}
export interface CollectionWorksAppendResponse {
  added: string[]
  skipped: string[]
  work_count: number
}
export interface Conversion_CommonSettings {
  delete_mode?: 'soft' | 'hard'
  matched: Conversion_CommonSettings_Reconcile_DesiredProfile
  mode?: 'strict' | 'available_sources'
  no_effect_tags: string[] | null
  other?: Conversion_CommonSettings_Reconcile_DesiredProfile
  other_tags: string[] | null
  unmatched: Conversion_CommonSettings_Reconcile_DesiredProfile
}
export interface Conversion_CommonSettings_Reconcile_DesiredProfile {
  encoded?: Conversion_CommonSettings_Reconcile_AudioOutputSpec
  lossless?: Conversion_CommonSettings_Reconcile_AudioOutputSpec
}
export interface Conversion_CommonSettings_Reconcile_AudioOutputSpec {
  codec: string
  quality?: Conversion_CommonSettings_Reconcile_Quality
}
export interface Conversion_CommonSettings_Reconcile_Quality {
  bitrate?: number
  kind: string
}
export interface Conversion_CommonSettingsRequest {
  delete_mode?: 'soft' | 'hard'
  matched?: Conversion_CommonSettingsRequest_Reconcile_DesiredProfile
  mode?: 'strict' | 'available_sources'
  no_effect_tags?: string[] | null
  other?: Conversion_CommonSettingsRequest_Reconcile_DesiredProfile
  other_tags?: string[] | null
  unmatched?: Conversion_CommonSettingsRequest_Reconcile_DesiredProfile
}
export interface Conversion_CommonSettingsRequest_Reconcile_DesiredProfile {
  encoded?: Conversion_CommonSettingsRequest_Reconcile_AudioOutputSpec
  lossless?: Conversion_CommonSettingsRequest_Reconcile_AudioOutputSpec
}
export interface Conversion_CommonSettingsRequest_Reconcile_AudioOutputSpec {
  codec?: string
  quality?: Conversion_CommonSettingsRequest_Reconcile_Quality
}
export interface Conversion_CommonSettingsRequest_Reconcile_Quality {
  bitrate?: number
  kind?: string
}
export interface Conversion_DraftDoc {
  delete_mode?: 'soft' | 'hard'
  matched: Conversion_DraftDoc_Reconcile_DesiredProfile
  members?: Conversion_DraftDoc_Conversion_DraftMember[]
  mode?: 'strict' | 'available_sources'
  no_effect_tags: string[] | null
  other?: Conversion_DraftDoc_Reconcile_DesiredProfile
  other_tags: string[] | null
  schema_version: number
  unmatched: Conversion_DraftDoc_Reconcile_DesiredProfile
}
export interface Conversion_DraftDoc_Reconcile_DesiredProfile {
  encoded?: Conversion_DraftDoc_Reconcile_AudioOutputSpec
  lossless?: Conversion_DraftDoc_Reconcile_AudioOutputSpec
}
export interface Conversion_DraftDoc_Reconcile_AudioOutputSpec {
  codec: string
  quality?: Conversion_DraftDoc_Reconcile_Quality
}
export interface Conversion_DraftDoc_Reconcile_Quality {
  bitrate?: number
  kind: string
}
export interface Conversion_DraftDoc_Conversion_DraftMember {
  excluded?: boolean
  member_id: string
  overrides?: Conversion_DraftDoc_Conversion_OverrideSet
}
export interface Conversion_DraftDoc_Conversion_OverrideSet {
  matched?: Conversion_DraftDoc_Reconcile_DesiredProfile
  mode?: 'strict' | 'available_sources'
  no_effect_tags?: string[] | null
  other?: Conversion_DraftDoc_Reconcile_DesiredProfile
  other_tags?: string[] | null
  unmatched?: Conversion_DraftDoc_Reconcile_DesiredProfile
}
export interface Conversion_DraftDocRequest {
  delete_mode?: 'soft' | 'hard'
  matched?: Conversion_DraftDocRequest_Reconcile_DesiredProfile
  members?: Conversion_DraftDocRequest_Conversion_DraftMember[]
  mode?: 'strict' | 'available_sources'
  no_effect_tags?: string[] | null
  other?: Conversion_DraftDocRequest_Reconcile_DesiredProfile
  other_tags?: string[] | null
  schema_version?: number
  unmatched?: Conversion_DraftDocRequest_Reconcile_DesiredProfile
}
export interface Conversion_DraftDocRequest_Reconcile_DesiredProfile {
  encoded?: Conversion_DraftDocRequest_Reconcile_AudioOutputSpec
  lossless?: Conversion_DraftDocRequest_Reconcile_AudioOutputSpec
}
export interface Conversion_DraftDocRequest_Reconcile_AudioOutputSpec {
  codec?: string
  quality?: Conversion_DraftDocRequest_Reconcile_Quality
}
export interface Conversion_DraftDocRequest_Reconcile_Quality {
  bitrate?: number
  kind?: string
}
export interface Conversion_DraftDocRequest_Conversion_DraftMember {
  excluded?: boolean
  member_id?: string
  overrides?: Conversion_DraftDocRequest_Conversion_OverrideSet
}
export interface Conversion_DraftDocRequest_Conversion_OverrideSet {
  matched?: Conversion_DraftDocRequest_Reconcile_DesiredProfile
  mode?: 'strict' | 'available_sources'
  no_effect_tags?: string[] | null
  other?: Conversion_DraftDocRequest_Reconcile_DesiredProfile
  other_tags?: string[] | null
  unmatched?: Conversion_DraftDocRequest_Reconcile_DesiredProfile
}
export interface Conversion_ExecutionOptions {
  delete_mode: 'soft' | 'hard'
}
export interface Conversion_FrozenMemberConclusion {
  matched: Conversion_FrozenMemberConclusion_Conversion_PartitionConclusion
  status: 'blocked' | 'source_ambiguous' | 'partially_unmet' | 'unmet' | 'changed' | 'not_applicable' | 'unchanged'
  unmatched: Conversion_FrozenMemberConclusion_Conversion_PartitionConclusion
}
export interface Conversion_FrozenMemberConclusion_Conversion_PartitionConclusion {
  applicable: boolean
  blocked: boolean
  changes: boolean
  unmet: boolean
}
export interface ConversionV1DraftResponse {
  operation_type: 'conversion' | 'rename' | 'metadata_refresh'
  task: ConversionV1DraftResponse_TaskDraftEnvelope
  updated_at: string
  version: number
  workset_id: string
}
export interface ConversionV1DraftResponse_TaskDraftEnvelope {
  kind: 'conversion'
  payload: Conversion_DraftDoc
  schema_version: 1
}
export interface ConversionV1ExecutionComponentResponse {
  committed: string[]
  completed_operations: number
  component_id: string
  component_index: number
  error_code?: string
  error_message?: string
  inventory_sync_error?: string
  inventory_synced: boolean
  kept_decisions: Reconcile_FileDecision[]
  operations: number
  partition: '' | 'matched' | 'unmatched'
  recovery: string[]
  remaining: string[]
  removed: string[]
  root_path: string
  stage?: 'precheck' | 'materialize' | 'validate' | 'commit' | 'remove' | 'skipped' | 'metadata'
  status: 'pending' | 'succeeded' | 'failed' | 'canceled'
}
export interface Reconcile_FileDecision {
  message?: string
  path: string
  reason_code?: string
  resolution: 'keep' | 'delete' | 'encode'
  target_path?: string
}
export interface ConversionV1ExecutionFullViewResponse {
  completed_components: number
  completed_operations: number
  components: ConversionV1ExecutionFullViewResponse_ExecutionComponentResponse[]
  created_at: string
  current_component_id: string
  current_phase: string
  current_root: string
  error_code: string
  error_message: string
  execution_id: string
  finished_at: string
  operation_type: 'conversion'
  options?: Conversion_ExecutionOptions
  plan_id: string
  selected_member_ids?: string[]
  started_at: string
  status: 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'interrupted'
  total_components: number
  total_operations: number
  workset_id: string
}
export interface ConversionV1ExecutionFullViewResponse_ExecutionComponentResponse {
  committed: string[]
  completed_operations: number
  component_id: string
  component_index: number
  error_code?: string
  error_message?: string
  inventory_sync_error?: string
  inventory_synced: boolean
  kept_decisions: Reconcile_FileDecision[]
  operations: number
  partition: '' | 'matched' | 'unmatched'
  recovery: string[]
  remaining: string[]
  removed: string[]
  root_path: string
  stage?: 'precheck' | 'materialize' | 'validate' | 'commit' | 'remove' | 'skipped' | 'metadata'
  status: 'pending' | 'succeeded' | 'failed' | 'canceled'
}
export interface ConversionV1ExecutionMemberViewResponse {
  completed_components: number
  completed_operations: number
  components: ConversionV1ExecutionMemberViewResponse_ExecutionComponentResponse[]
  created_at: string
  current_component_id: string
  current_phase: string
  current_root: string
  error_code: string
  error_message: string
  execution_id: string
  finished_at: string
  member: ConversionV1ExecutionMemberViewResponse_ExecutionMemberSummaryResponse
  operation_type: 'conversion'
  options?: Conversion_ExecutionOptions
  plan_id: string
  selected_member_ids?: string[]
  started_at: string
  status: 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'interrupted'
  total_components: number
  total_operations: number
  workset_id: string
}
export interface ConversionV1ExecutionMemberViewResponse_ExecutionComponentResponse {
  committed: string[]
  completed_operations: number
  component_id: string
  component_index: number
  error_code?: string
  error_message?: string
  inventory_sync_error?: string
  inventory_synced: boolean
  kept_decisions: Reconcile_FileDecision[]
  operations: number
  partition: '' | 'matched' | 'unmatched'
  recovery: string[]
  remaining: string[]
  removed: string[]
  root_path: string
  stage?: 'precheck' | 'materialize' | 'validate' | 'commit' | 'remove' | 'skipped' | 'metadata'
  status: 'pending' | 'succeeded' | 'failed' | 'canceled'
}
export interface ConversionV1ExecutionMemberViewResponse_ExecutionMemberSummaryResponse {
  completed_components: number
  completed_operations: number
  error_message: string
  inventory_sync_error: string
  member_id: string
  root_path: string
  status: 'pending' | 'running' | 'succeeded' | 'failed' | 'canceled'
  total_components: number
  total_operations: number
}
export interface ConversionV1ExecutionMembersViewResponse {
  completed_components: number
  completed_operations: number
  created_at: string
  current_component_id: string
  current_phase: string
  current_root: string
  error_code: string
  error_message: string
  execution_id: string
  finished_at: string
  members: ConversionV1ExecutionMembersViewResponse_ExecutionMembersResponse
  operation_type: 'conversion'
  options?: Conversion_ExecutionOptions
  plan_id: string
  selected_member_ids?: string[]
  started_at: string
  status: 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'interrupted'
  total_components: number
  total_operations: number
  workset_id: string
}
export interface ConversionV1ExecutionMembersViewResponse_ExecutionMembersResponse {
  attention: ConversionV1ExecutionMembersViewResponse_ExecutionMemberSummaryResponse[]
  attention_total: number
  counts: ConversionV1ExecutionMembersViewResponse_ExecutionMemberCountsResponse
}
export interface ConversionV1ExecutionMembersViewResponse_ExecutionMemberSummaryResponse {
  completed_components: number
  completed_operations: number
  error_message: string
  inventory_sync_error: string
  member_id: string
  root_path: string
  status: 'pending' | 'running' | 'succeeded' | 'failed' | 'canceled'
  total_components: number
  total_operations: number
}
export interface ConversionV1ExecutionMembersViewResponse_ExecutionMemberCountsResponse {
  canceled: number
  failed: number
  pending: number
  running: number
  succeeded: number
}
export interface ConversionV1ExecutionProgressResponse {
  completed_components: number
  completed_operations: number
  current_component_id: string
  current_phase: string
  current_root: string
  execution_id: string
  options?: Conversion_ExecutionOptions
  plan_id: string
  status: 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'interrupted'
  total_components: number
  total_operations: number
}
export interface ConversionV1ExecutionStartResponse {
  created: boolean
  execution: ConversionV1ExecutionStartResponse_ExecutionViewResponse
}
export interface ConversionV1ExecutionStartResponse_ExecutionComponentResponse {
  committed: string[]
  completed_operations: number
  component_id: string
  component_index: number
  error_code?: string
  error_message?: string
  inventory_sync_error?: string
  inventory_synced: boolean
  kept_decisions: Reconcile_FileDecision[]
  operations: number
  partition: '' | 'matched' | 'unmatched'
  recovery: string[]
  remaining: string[]
  removed: string[]
  root_path: string
  stage?: 'precheck' | 'materialize' | 'validate' | 'commit' | 'remove' | 'skipped' | 'metadata'
  status: 'pending' | 'succeeded' | 'failed' | 'canceled'
}
export interface ConversionV1ExecutionStartResponse_ExecutionMembersResponse {
  attention: ConversionV1ExecutionStartResponse_ExecutionMemberSummaryResponse[]
  attention_total: number
  counts: ConversionV1ExecutionStartResponse_ExecutionMemberCountsResponse
}
export interface ConversionV1ExecutionStartResponse_ExecutionMemberSummaryResponse {
  completed_components: number
  completed_operations: number
  error_message: string
  inventory_sync_error: string
  member_id: string
  root_path: string
  status: 'pending' | 'running' | 'succeeded' | 'failed' | 'canceled'
  total_components: number
  total_operations: number
}
export interface ConversionV1ExecutionStartResponse_ExecutionMemberCountsResponse {
  canceled: number
  failed: number
  pending: number
  running: number
  succeeded: number
}
export interface ConversionV1ExecutionViewResponse_ExecutionComponentResponse {
  committed: string[]
  completed_operations: number
  component_id: string
  component_index: number
  error_code?: string
  error_message?: string
  inventory_sync_error?: string
  inventory_synced: boolean
  kept_decisions: Reconcile_FileDecision[]
  operations: number
  partition: '' | 'matched' | 'unmatched'
  recovery: string[]
  remaining: string[]
  removed: string[]
  root_path: string
  stage?: 'precheck' | 'materialize' | 'validate' | 'commit' | 'remove' | 'skipped' | 'metadata'
  status: 'pending' | 'succeeded' | 'failed' | 'canceled'
}
export interface ConversionV1ExecutionViewResponse_ExecutionMembersResponse {
  attention: ConversionV1ExecutionViewResponse_ExecutionMemberSummaryResponse[]
  attention_total: number
  counts: ConversionV1ExecutionViewResponse_ExecutionMemberCountsResponse
}
export interface ConversionV1ExecutionViewResponse_ExecutionMemberSummaryResponse {
  completed_components: number
  completed_operations: number
  error_message: string
  inventory_sync_error: string
  member_id: string
  root_path: string
  status: 'pending' | 'running' | 'succeeded' | 'failed' | 'canceled'
  total_components: number
  total_operations: number
}
export interface ConversionV1ExecutionViewResponse_ExecutionMemberCountsResponse {
  canceled: number
  failed: number
  pending: number
  running: number
  succeeded: number
}
export interface ConversionV1MemberConclusionResponse {
  category?: string
  excluded: boolean
  has_components: boolean
  input: 'none' | 'valid' | 'stale' | 'missing' | 'unavailable'
  member_id: string
  task?: Conversion_FrozenMemberConclusion
}
export interface ConversionV1MemberReviewResponse {
  conclusion: ConversionV1MemberReviewResponse_MemberConclusionResponse
  effective: Reconcile_Policy | null
  excluded: boolean
  folder_path: string
  member_id: string
  member_name: string
  plan_id: string
  roots: ConversionV1MemberReviewResponse_RootValidationResponse[]
  sources: {
    [k: string]: string
  } | null
  task: ConversionV1MemberReviewResponse_TaskEnvelopeResponse
}
export interface ConversionV1MemberReviewResponse_MemberConclusionResponse {
  category?: string
  excluded: boolean
  has_components: boolean
  input: 'none' | 'valid' | 'stale' | 'missing' | 'unavailable'
  member_id: string
  task?: Conversion_FrozenMemberConclusion
}
export interface Reconcile_Policy {
  matched: Reconcile_Policy_Reconcile_DesiredProfile
  mode?: 'strict' | 'available_sources'
  no_effect_tags?: string[]
  other?: Reconcile_Policy_Reconcile_DesiredProfile
  other_tags: string[] | null
  schema_version: number
  unmatched: Reconcile_Policy_Reconcile_DesiredProfile
}
export interface Reconcile_Policy_Reconcile_DesiredProfile {
  encoded?: Reconcile_Policy_Reconcile_AudioOutputSpec
  lossless?: Reconcile_Policy_Reconcile_AudioOutputSpec
}
export interface Reconcile_Policy_Reconcile_AudioOutputSpec {
  codec: string
  quality?: Reconcile_Policy_Reconcile_Quality
}
export interface Reconcile_Policy_Reconcile_Quality {
  bitrate?: number
  kind: string
}
export interface ConversionV1MemberReviewResponse_RootValidationResponse {
  entry_count: number
  inventory_fingerprint: string
  root_error_code: string
  root_error_message: string
  root_index: number
  root_path: string
  root_status: string
  stale: boolean
}
export interface ConversionV1MemberReviewResponse_TaskEnvelopeResponse {
  kind: 'conversion'
  payload: ConversionV1MemberReviewResponse_TaskPayloadResponse
  schema_version: 1
}
export interface ConversionV1MemberReviewResponse_TaskPayloadResponse {
  classifier: TaskClassifierSnapshot
  components: Reconcile_ComponentOutcome[]
  policy: Reconcile_Policy
  policy_hash: string
  summary: Reconcile_StepSummary
}
export interface TaskClassifierSnapshot {
  hash: string
  tags: string[]
}
export interface Reconcile_ComponentOutcome {
  component_id: string
  files: Reconcile_ComponentOutcome_Reconcile_FileTuple[]
  lanes: Reconcile_ComponentOutcome_Reconcile_LaneDecision[]
  message?: string
  operations: Reconcile_ComponentOutcome_Reconcile_Operation[]
  partition: 'matched' | 'unmatched'
  projected_inventory: string[]
  reason_code?: string
  scope?: string
  status: 'ok' | 'blocked'
  variant_decisions: Reconcile_ComponentOutcome_Reconcile_VariantDecision[]
}
export interface Reconcile_ComponentOutcome_Reconcile_FileTuple {
  Mtime: number
  Path: string
  Size: number
}
export interface Reconcile_ComponentOutcome_Reconcile_LaneDecision {
  decision: 'KEEP_ALL' | 'REBUILD_ALL' | 'REBUILD' | 'KEEP' | 'BLOCKED'
  lane: 'lossless' | 'encoded'
  message?: string
  reason_code?: string
}
export interface Reconcile_ComponentOutcome_Reconcile_Operation {
  component_id: string
  depends_on?: string[]
  kind: 'encode' | 'delete_obsolete'
  phase: 'materialize_outputs' | 'validate_outputs' | 'commit_outputs' | 'remove_obsolete_audio'
  source_path: string
  target_path?: string
  variant_stem: string
}
export interface Reconcile_ComponentOutcome_Reconcile_VariantDecision {
  decisions: Reconcile_ComponentOutcome_Reconcile_FileDecision[]
  stem: string
}
export interface Reconcile_ComponentOutcome_Reconcile_FileDecision {
  message?: string
  path: string
  reason_code?: string
  resolution: 'keep' | 'delete' | 'encode'
  target_path?: string
}
export interface Reconcile_StepSummary {
  blocked_count: number
  component_count: number
  error_count: number
  operation_count: number
  summary_reason: string
  unmet_targets?: number
}
export interface ConversionV1OperationResponse {
  active_execution: ConversionV1OperationResponse_ExecutionProgressResponse | null
  active_generation: ConversionV1OperationResponse_GenerationProgressResponse | null
  current_revision: ConversionV1OperationResponse_CurrentRevisionResponse | null
  latest_execution: ConversionV1OperationResponse_ExecutionRefResponse | null
  latest_generation: ConversionV1OperationResponse_GenerationSummaryResponse | null
  max_participants?: number
  operation_type: 'conversion' | 'rename' | 'metadata_refresh'
  planning_state: 'unplanned' | 'planned' | 'needs_planning' | 'planning' | 'orphaned'
  version: number
  workset_id: string
}
export interface ConversionV1OperationResponse_ExecutionProgressResponse {
  completed_components: number
  completed_operations: number
  current_component_id: string
  current_phase: string
  current_root: string
  execution_id: string
  options?: Conversion_ExecutionOptions
  plan_id: string
  status: 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'interrupted'
  total_components: number
  total_operations: number
}
export interface ConversionV1OperationResponse_GenerationProgressResponse {
  completed_roots: number
  current_root: string
  error_count: number
  generation_id: string
  status: 'queued' | 'running' | 'completed' | 'failed' | 'canceled' | 'interrupted'
  total_roots: number
}
export interface ConversionV1OperationResponse_CurrentRevisionResponse {
  counts: ConversionV1OperationResponse_RevisionCountsResponse
  created_at: string
  plan_id: string
  revision_index: number
  stale: boolean | null
  status: string
  summary_reason: string
  validation_state: 'valid' | 'stale' | 'unavailable'
}
export interface ConversionV1OperationResponse_RevisionCountsResponse {
  blocked: number
  changed: number
  members: number
  unchanged: number
  unmet_targets: number
}
export interface ConversionV1OperationResponse_ExecutionRefResponse {
  error_code?: string
  error_message?: string
  execution_id: string
  finished_at?: string
  plan_id: string
  status: 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'interrupted'
}
export interface ConversionV1OperationResponse_GenerationSummaryResponse {
  error_code: string
  error_message: string
  finished_at: string
  generation_id: string
  status: 'queued' | 'running' | 'completed' | 'failed' | 'canceled' | 'interrupted'
}
export interface ConversionV1RevisionMemberResponse {
  conclusion: ConversionV1RevisionMemberResponse_MemberConclusionResponse
  effective: Reconcile_Policy | null
  excluded: boolean
  folder_path: string
  member_id: string
  member_name: string
  sources: {
    [k: string]: string
  } | null
}
export interface ConversionV1RevisionMemberResponse_MemberConclusionResponse {
  category?: string
  excluded: boolean
  has_components: boolean
  input: 'none' | 'valid' | 'stale' | 'missing' | 'unavailable'
  member_id: string
  task?: Conversion_FrozenMemberConclusion
}
export interface ConversionV1RevisionResponse {
  category_counts: {
    [k: string]: number
  } | null
  category_total?: number
  counts: ConversionV1RevisionResponse_RevisionCountsResponse
  created_at: string
  execution: ConversionV1RevisionResponse_ExecutionRefResponse | null
  members: ConversionV1RevisionResponse_MemberConclusionResponse[]
  plan_id: string
  revision_index: number
  root_path: string
  snapshot_token: string
  status: string
  summary: ConversionV1RevisionResponse_PlanSummaryResponse
  task: ConversionV1RevisionResponse_TaskEnvelopeResponse
}
export interface ConversionV1RevisionResponse_RevisionCountsResponse {
  blocked: number
  changed: number
  members: number
  unchanged: number
  unmet_targets: number
}
export interface ConversionV1RevisionResponse_ExecutionRefResponse {
  error_code?: string
  error_message?: string
  execution_id: string
  finished_at?: string
  plan_id: string
  status: 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'interrupted'
}
export interface ConversionV1RevisionResponse_MemberConclusionResponse {
  category?: string
  excluded: boolean
  has_components: boolean
  input: 'none' | 'valid' | 'stale' | 'missing' | 'unavailable'
  member_id: string
  task?: Conversion_FrozenMemberConclusion
}
export interface ConversionV1RevisionResponse_PlanSummaryResponse {
  actionable_count: number
  error_count: number
  operation_count: number
  summary_reason: string
  total_count: number
}
export interface ConversionV1RevisionResponse_TaskEnvelopeResponse {
  kind: 'conversion'
  payload: ConversionV1RevisionResponse_TaskPayloadResponse
  schema_version: 1
}
export interface ConversionV1RevisionResponse_TaskPayloadResponse {
  classifier: TaskClassifierSnapshot
  policy: Reconcile_Policy
  policy_hash: string
  summary: Reconcile_StepSummary
}
export interface ConversionV1TaskDraftEnvelope {
  kind: 'conversion'
  payload: Conversion_DraftDoc
  schema_version: 1
}
export interface ConversionV1TaskEnvelopeResponse {
  kind: 'conversion'
  payload: ConversionV1TaskEnvelopeResponse_TaskPayloadResponse
  schema_version: 1
}
export interface ConversionV1TaskEnvelopeResponse_TaskPayloadResponse {
  classifier: TaskClassifierSnapshot
  components?: Reconcile_ComponentOutcome[]
  policy: Reconcile_Policy
  policy_hash: string
  summary: Reconcile_StepSummary
}
export interface ConversionV1TaskPayloadResponse {
  classifier: TaskClassifierSnapshot
  components?: Reconcile_ComponentOutcome[]
  policy: Reconcile_Policy
  policy_hash: string
  summary: Reconcile_StepSummary
}
export interface ConversionV1TaskSettingsRequest {
  settings?: Conversion_CommonSettingsRequest
}
export interface ConversionV1TaskSettingsResponse {
  operation_type: 'conversion'
  settings: Conversion_CommonSettings
}
export interface CopyResponse {
  copy_id: string
  dir_id: string
  library_id: string
  rel_path: string
}
export interface CoverDestinationResponse {
  digest?: string
  error_code?: string
  error_message?: string
  execution_id: string
  id: string
  published?: string
  refresh: boolean
  status: 'pending' | 'running' | 'succeeded' | 'skipped' | 'failed' | 'canceled'
  target: CoverDestinationResponse_CoverTargetResponse
}
export interface CoverDestinationResponse_CoverTargetResponse {
  copy_id: string
  identity: string
  library_root: string
  rel_path: string
  rj: string
}
export interface CoverReportResponse {
  counts: {
    [k: string]: number
  } | null
  items: CoverReportResponse_CoverDestinationResponse[]
}
export interface CoverReportResponse_CoverDestinationResponse {
  digest?: string
  error_code?: string
  error_message?: string
  execution_id: string
  id: string
  published?: string
  refresh: boolean
  status: 'pending' | 'running' | 'succeeded' | 'skipped' | 'failed' | 'canceled'
  target: CoverReportResponse_CoverTargetResponse
}
export interface CoverReportResponse_CoverTargetResponse {
  copy_id: string
  identity: string
  library_root: string
  rel_path: string
  rj: string
}
export interface CoverTargetResponse {
  copy_id: string
  identity: string
  library_root: string
  rel_path: string
  rj: string
}
export interface CurrentRecordResponse {
  workset: CurrentRecordResponse_WorksetResponse | null
}
export interface CurrentRecordResponse_WorksetResponse {
  created_at: string
  library: CurrentRecordResponse_LibraryRefResponse | null
  members: CurrentRecordResponse_MemberResponse[]
  title: string
  updated_at: string
  version: number
  workset_id: string
}
export interface CurrentRecordResponse_LibraryRefResponse {
  library_id: string
  name: string
  root_path: string
}
export interface CurrentRecordResponse_MemberResponse {
  dir_id: string
  folder_name: string
  folder_path: string
  member_id: string
  rel_path: string
}
export interface CurrentRevisionResponse {
  counts: CurrentRevisionResponse_RevisionCountsResponse
  created_at: string
  plan_id: string
  revision_index: number
  stale: boolean | null
  status: string
  summary_reason: string
  validation_state: 'valid' | 'stale' | 'unavailable'
}
export interface CurrentRevisionResponse_RevisionCountsResponse {
  blocked: number
  changed: number
  members: number
  unchanged: number
  unmet_targets: number
}
export interface DesiredProfileDTO {
  encoded?: DesiredProfileDTO_AudioOutputSpecDTO
  lossless?: DesiredProfileDTO_AudioOutputSpecDTO
}
export interface DesiredProfileDTO_AudioOutputSpecDTO {
  codec: string
  quality?: DesiredProfileDTO_AudioQualityDTO
}
export interface DesiredProfileDTO_AudioQualityDTO {
  bitrate?: number
  kind: string
}
export interface DirListResponse {
  dirs: DirListResponse_DirResponse[]
}
export interface DirListResponse_DirResponse {
  added_at: string
  audio_file_count: number
  dir_id: string
  file_count: number
  name: string
  path: string
  rel_path: string
  work_code: string | null
}
export interface DirResponse {
  added_at: string
  audio_file_count: number
  dir_id: string
  file_count: number
  name: string
  path: string
  rel_path: string
  work_code: string | null
}
export interface Dirrename_Common {
  strip_book_title?: boolean
  strip_bracketed_title?: boolean
  strip_tilde_title?: boolean
  template: string
  voice_actor_separator?: string
}
export interface Dirrename_CommonRequest {
  strip_book_title?: boolean
  strip_bracketed_title?: boolean
  strip_tilde_title?: boolean
  template?: string
  voice_actor_separator?: string
}
export interface Dirrename_Draft {
  common: Dirrename_Draft_Dirrename_Common
  members: Dirrename_Draft_Dirrename_Member[] | null
  schema_version: number
}
export interface Dirrename_Draft_Dirrename_Common {
  strip_book_title?: boolean
  strip_bracketed_title?: boolean
  strip_tilde_title?: boolean
  template: string
  voice_actor_separator?: string
}
export interface Dirrename_Draft_Dirrename_Member {
  excluded: boolean
  member_id: string
  name?: string
}
export interface Dirrename_DraftRequest {
  common?: Dirrename_DraftRequest_Dirrename_Common
  members?: Dirrename_DraftRequest_Dirrename_Member[] | null
  schema_version?: number
}
export interface Dirrename_DraftRequest_Dirrename_Common {
  strip_book_title?: boolean
  strip_bracketed_title?: boolean
  strip_tilde_title?: boolean
  template?: string
  voice_actor_separator?: string
}
export interface Dirrename_DraftRequest_Dirrename_Member {
  excluded?: boolean
  member_id?: string
  name?: string
}
export interface Dirrename_Outcome {
  action: 'rename' | 'noop' | 'blocked'
  custom_name?: string
  member_id: string
  observed_name?: string
  raw_name: string
  reason_code?: string
  rebind_to?: string
  source_name: string
  target_name: string
  work_code: string
}
export interface Dirrename_Result {
  recovered?: boolean
  renamed: boolean
  rolled_back?: boolean
  skipped?: string
  source_path: string
  target_path: string
}
export interface DraftDocumentRequest {
  delete_mode?: '' | 'soft' | 'hard'
  matched?: DraftDocumentRequest_DesiredProfileDTO
  members?: DraftDocumentRequest_DraftMemberRequest[]
  mode?: '' | 'strict' | 'available_sources'
  no_effect_tags?: string[]
  other?: DraftDocumentRequest_DesiredProfileDTO
  other_tags?: string[]
  schema_version?: number
  unmatched?: DraftDocumentRequest_DesiredProfileDTO
}
export interface DraftDocumentRequest_DesiredProfileDTO {
  encoded?: DraftDocumentRequest_AudioOutputSpecDTO
  lossless?: DraftDocumentRequest_AudioOutputSpecDTO
}
export interface DraftDocumentRequest_AudioOutputSpecDTO {
  codec?: string
  quality?: DraftDocumentRequest_AudioQualityDTO
}
export interface DraftDocumentRequest_AudioQualityDTO {
  bitrate?: number
  kind?: string
}
export interface DraftDocumentRequest_DraftMemberRequest {
  excluded?: boolean
  member_id?: string
  overrides?: DraftDocumentRequest_DraftOverridesRequest | null
}
export interface DraftDocumentRequest_DraftOverridesRequest {
  matched?: DraftDocumentRequest_DesiredProfileDTO | null
  mode?: ('strict' | 'available_sources') | null
  no_effect_tags?: string[] | null
  other?: DraftDocumentRequest_DesiredProfileDTO | null
  other_tags?: string[] | null
  unmatched?: DraftDocumentRequest_DesiredProfileDTO | null
}
export interface DraftMemberRequest {
  excluded?: boolean
  member_id?: string
  overrides?: DraftMemberRequest_DraftOverridesRequest | null
}
export interface DraftMemberRequest_DraftOverridesRequest {
  matched?: DraftMemberRequest_DesiredProfileDTO | null
  mode?: ('strict' | 'available_sources') | null
  no_effect_tags?: string[] | null
  other?: DraftMemberRequest_DesiredProfileDTO | null
  other_tags?: string[] | null
  unmatched?: DraftMemberRequest_DesiredProfileDTO | null
}
export interface DraftMemberRequest_DesiredProfileDTO {
  encoded?: DraftMemberRequest_AudioOutputSpecDTO
  lossless?: DraftMemberRequest_AudioOutputSpecDTO
}
export interface DraftMemberRequest_AudioOutputSpecDTO {
  codec?: string
  quality?: DraftMemberRequest_AudioQualityDTO
}
export interface DraftMemberRequest_AudioQualityDTO {
  bitrate?: number
  kind?: string
}
export interface DraftOverridesRequest {
  matched?: DraftOverridesRequest_DesiredProfileDTO | null
  mode?: ('strict' | 'available_sources') | null
  no_effect_tags?: string[] | null
  other?: DraftOverridesRequest_DesiredProfileDTO | null
  other_tags?: string[] | null
  unmatched?: DraftOverridesRequest_DesiredProfileDTO | null
}
export interface DraftOverridesRequest_DesiredProfileDTO {
  encoded?: DraftOverridesRequest_AudioOutputSpecDTO
  lossless?: DraftOverridesRequest_AudioOutputSpecDTO
}
export interface DraftOverridesRequest_AudioOutputSpecDTO {
  codec?: string
  quality?: DraftOverridesRequest_AudioQualityDTO
}
export interface DraftOverridesRequest_AudioQualityDTO {
  bitrate?: number
  kind?: string
}
export interface RenameV1DraftResponse {
  operation_type: 'conversion' | 'rename' | 'metadata_refresh'
  task: RenameV1DraftResponse_TaskDraftEnvelope
  updated_at: string
  version: number
  workset_id: string
}
export interface RenameV1DraftResponse_TaskDraftEnvelope {
  kind: 'rename'
  payload: Dirrename_Draft
  schema_version: 1
}
export interface MetadataRefreshV1DraftResponse {
  operation_type: 'conversion' | 'rename' | 'metadata_refresh'
  task: MetadataRefreshV1DraftResponse_TaskDraftEnvelope
  updated_at: string
  version: number
  workset_id: string
}
export interface MetadataRefreshV1DraftResponse_TaskDraftEnvelope {
  kind: 'metadata_refresh'
  payload: Refresh_Draft
  schema_version: 1
}
export interface Refresh_Draft {
  common: Refresh_Draft_Refresh_Common
  members: Refresh_Draft_Refresh_Member[] | null
  schema_version: number
}
export interface Refresh_Draft_Refresh_Common {
  download_cover: boolean
  locale: 'ja-jp' | 'zh-cn' | 'zh-tw'
  mode: 'fill_missing' | 'refresh_dynamic' | 'refresh_all'
}
export interface Refresh_Draft_Refresh_Member {
  excluded: boolean
  member_id: string
}
export interface EditionResponse {
  display_order: number
  edition_id: string | null
  label: string | null
  language: string
  version_root_rj: string
}
export interface EditionSlotResponse {
  held: boolean
  label: string | null
  language: string
  root: EditionSlotResponse_TreeNodeResponse
  translations: EditionSlotResponse_TreeNodeResponse[]
}
export interface EditionSlotResponse_TreeNodeResponse {
  held: boolean
  is_work: boolean
  language: string | null
  relation: EditionSlotResponse_RelationResponse
  rj: string
  role: '' | 'original' | 'parent' | 'child' | 'edition_page' | 'translation'
  source_url: string | null
}
export interface EditionSlotResponse_RelationResponse {
  language: string | null
  original_rj: string | null
  parent_rj: string | null
  role: ('original' | 'parent' | 'child') | null
}
export interface EmptyEvent {}
export interface EmptyRequest {}
export interface ErrorResponse {
  code: string
  details?: string[]
  message: string
}
export interface RenameV1ExecutionComponentResponse {
  committed: string[]
  completed_operations: number
  component_id: string
  component_index: number
  error_code?: string
  error_message?: string
  inventory_sync_error?: string
  inventory_synced: boolean
  /**
   * @maxItems 0
   */
  kept_decisions: []
  operations: number
  partition: '' | 'matched' | 'unmatched'
  recovery: string[]
  remaining: string[]
  removed: string[]
  root_path: string
  stage?: 'precheck' | 'materialize' | 'validate' | 'commit' | 'remove' | 'skipped' | 'metadata'
  status: 'pending' | 'succeeded' | 'failed' | 'canceled'
  task_result?: Dirrename_Result
}
export interface Refresh_Outcome {
  action: 'refresh' | 'skip' | 'cover'
  member_id: string
  reason_code?: string
  target: Refresh_Outcome_Coverdelivery_Target
}
export interface Refresh_Outcome_Coverdelivery_Target {
  copy_id: string
  identity: string
  library_root: string
  rel_path: string
  rj: string
}
export interface Refresh_WorkResult {
  backup_url?: string
  issues: Refresh_WorkResult_Refresh_Issue[] | null
  main_url?: string
  outcome: Refresh_WorkResult_Works_ImportOutcome
  rj: string
}
export interface Refresh_WorkResult_Refresh_Issue {
  code: string
  group?: string
  message: string
  source?: string
}
export interface Refresh_WorkResult_Works_ImportOutcome {
  applied: Refresh_WorkResult_Works_AppliedGroup[] | null
  ignored: Refresh_WorkResult_Works_IgnoredGroup[] | null
  warnings: Refresh_WorkResult_Works_RelationWarning[] | null
}
export interface Refresh_WorkResult_Works_AppliedGroup {
  data_group: string
  dl_count?: number
}
export interface Refresh_WorkResult_Works_IgnoredGroup {
  data_group: string
  message: string
  reason: string
}
export interface Refresh_WorkResult_Works_RelationWarning {
  code: string
  message: string
  rj: string
}
export interface ExecutionFailedEvent {
  error_code: string
  error_message: string
  execution_id: string
}
export interface RenameV1ExecutionFullViewResponse {
  completed_components: number
  completed_operations: number
  components: RenameV1ExecutionFullViewResponse_ExecutionComponentResponse[]
  created_at: string
  current_component_id: string
  current_phase: string
  current_root: string
  error_code: string
  error_message: string
  execution_id: string
  finished_at: string
  operation_type: 'rename'
  plan_id: string
  selected_member_ids?: string[]
  started_at: string
  status: 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'interrupted'
  total_components: number
  total_operations: number
  workset_id: string
}
export interface RenameV1ExecutionFullViewResponse_ExecutionComponentResponse {
  committed: string[]
  completed_operations: number
  component_id: string
  component_index: number
  error_code?: string
  error_message?: string
  inventory_sync_error?: string
  inventory_synced: boolean
  /**
   * @maxItems 0
   */
  kept_decisions: []
  operations: number
  partition: '' | 'matched' | 'unmatched'
  recovery: string[]
  remaining: string[]
  removed: string[]
  root_path: string
  stage?: 'precheck' | 'materialize' | 'validate' | 'commit' | 'remove' | 'skipped' | 'metadata'
  status: 'pending' | 'succeeded' | 'failed' | 'canceled'
  task_result?: Dirrename_Result
}
export interface MetadataRefreshV1ExecutionFullViewResponse {
  completed_components: number
  completed_operations: number
  components: MetadataRefreshV1ExecutionFullViewResponse_ExecutionComponentResponse[]
  created_at: string
  current_component_id: string
  current_phase: string
  current_root: string
  error_code: string
  error_message: string
  execution_id: string
  finished_at: string
  operation_type: 'metadata_refresh'
  options?: Refresh_Common
  plan_id: string
  selected_member_ids?: string[]
  started_at: string
  status: 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'interrupted'
  total_components: number
  total_operations: number
  workset_id: string
}
export interface Refresh_Common {
  download_cover: boolean
  locale: 'ja-jp' | 'zh-cn' | 'zh-tw'
  mode: 'fill_missing' | 'refresh_dynamic' | 'refresh_all'
}
export interface ExecutionMemberCountsResponse {
  canceled: number
  failed: number
  pending: number
  running: number
  succeeded: number
}
export interface ExecutionMemberEvent {
  attention_total: number
  counts: ExecutionMemberEvent_ExecutionMemberCountsResponse
  member: ExecutionMemberEvent_ExecutionMemberSummaryResponse
}
export interface ExecutionMemberEvent_ExecutionMemberCountsResponse {
  canceled: number
  failed: number
  pending: number
  running: number
  succeeded: number
}
export interface ExecutionMemberEvent_ExecutionMemberSummaryResponse {
  completed_components: number
  completed_operations: number
  error_message: string
  inventory_sync_error: string
  member_id: string
  root_path: string
  status: 'pending' | 'running' | 'succeeded' | 'failed' | 'canceled'
  total_components: number
  total_operations: number
}
export interface ExecutionMemberSummaryResponse {
  completed_components: number
  completed_operations: number
  error_message: string
  inventory_sync_error: string
  member_id: string
  root_path: string
  status: 'pending' | 'running' | 'succeeded' | 'failed' | 'canceled'
  total_components: number
  total_operations: number
}
export interface RenameV1ExecutionMemberViewResponse {
  completed_components: number
  completed_operations: number
  components: RenameV1ExecutionMemberViewResponse_ExecutionComponentResponse[]
  created_at: string
  current_component_id: string
  current_phase: string
  current_root: string
  error_code: string
  error_message: string
  execution_id: string
  finished_at: string
  member: RenameV1ExecutionMemberViewResponse_ExecutionMemberSummaryResponse
  operation_type: 'rename'
  plan_id: string
  selected_member_ids?: string[]
  started_at: string
  status: 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'interrupted'
  total_components: number
  total_operations: number
  workset_id: string
}
export interface RenameV1ExecutionMemberViewResponse_ExecutionComponentResponse {
  committed: string[]
  completed_operations: number
  component_id: string
  component_index: number
  error_code?: string
  error_message?: string
  inventory_sync_error?: string
  inventory_synced: boolean
  /**
   * @maxItems 0
   */
  kept_decisions: []
  operations: number
  partition: '' | 'matched' | 'unmatched'
  recovery: string[]
  remaining: string[]
  removed: string[]
  root_path: string
  stage?: 'precheck' | 'materialize' | 'validate' | 'commit' | 'remove' | 'skipped' | 'metadata'
  status: 'pending' | 'succeeded' | 'failed' | 'canceled'
  task_result?: Dirrename_Result
}
export interface RenameV1ExecutionMemberViewResponse_ExecutionMemberSummaryResponse {
  completed_components: number
  completed_operations: number
  error_message: string
  inventory_sync_error: string
  member_id: string
  root_path: string
  status: 'pending' | 'running' | 'succeeded' | 'failed' | 'canceled'
  total_components: number
  total_operations: number
}
export interface MetadataRefreshV1ExecutionMemberViewResponse {
  completed_components: number
  completed_operations: number
  components: MetadataRefreshV1ExecutionMemberViewResponse_ExecutionComponentResponse[]
  created_at: string
  current_component_id: string
  current_phase: string
  current_root: string
  error_code: string
  error_message: string
  execution_id: string
  finished_at: string
  member: MetadataRefreshV1ExecutionMemberViewResponse_ExecutionMemberSummaryResponse
  operation_type: 'metadata_refresh'
  options?: Refresh_Common
  plan_id: string
  selected_member_ids?: string[]
  started_at: string
  status: 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'interrupted'
  total_components: number
  total_operations: number
  workset_id: string
}
export interface MetadataRefreshV1ExecutionMemberViewResponse_ExecutionMemberSummaryResponse {
  completed_components: number
  completed_operations: number
  error_message: string
  inventory_sync_error: string
  member_id: string
  root_path: string
  status: 'pending' | 'running' | 'succeeded' | 'failed' | 'canceled'
  total_components: number
  total_operations: number
}
export interface ExecutionMembersResponse {
  attention: ExecutionMembersResponse_ExecutionMemberSummaryResponse[]
  attention_total: number
  counts: ExecutionMembersResponse_ExecutionMemberCountsResponse
}
export interface ExecutionMembersResponse_ExecutionMemberSummaryResponse {
  completed_components: number
  completed_operations: number
  error_message: string
  inventory_sync_error: string
  member_id: string
  root_path: string
  status: 'pending' | 'running' | 'succeeded' | 'failed' | 'canceled'
  total_components: number
  total_operations: number
}
export interface ExecutionMembersResponse_ExecutionMemberCountsResponse {
  canceled: number
  failed: number
  pending: number
  running: number
  succeeded: number
}
export interface RenameV1ExecutionMembersViewResponse {
  completed_components: number
  completed_operations: number
  created_at: string
  current_component_id: string
  current_phase: string
  current_root: string
  error_code: string
  error_message: string
  execution_id: string
  finished_at: string
  members: RenameV1ExecutionMembersViewResponse_ExecutionMembersResponse
  operation_type: 'rename'
  plan_id: string
  selected_member_ids?: string[]
  started_at: string
  status: 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'interrupted'
  total_components: number
  total_operations: number
  workset_id: string
}
export interface RenameV1ExecutionMembersViewResponse_ExecutionMembersResponse {
  attention: RenameV1ExecutionMembersViewResponse_ExecutionMemberSummaryResponse[]
  attention_total: number
  counts: RenameV1ExecutionMembersViewResponse_ExecutionMemberCountsResponse
}
export interface RenameV1ExecutionMembersViewResponse_ExecutionMemberSummaryResponse {
  completed_components: number
  completed_operations: number
  error_message: string
  inventory_sync_error: string
  member_id: string
  root_path: string
  status: 'pending' | 'running' | 'succeeded' | 'failed' | 'canceled'
  total_components: number
  total_operations: number
}
export interface RenameV1ExecutionMembersViewResponse_ExecutionMemberCountsResponse {
  canceled: number
  failed: number
  pending: number
  running: number
  succeeded: number
}
export interface MetadataRefreshV1ExecutionMembersViewResponse {
  completed_components: number
  completed_operations: number
  created_at: string
  current_component_id: string
  current_phase: string
  current_root: string
  error_code: string
  error_message: string
  execution_id: string
  finished_at: string
  members: MetadataRefreshV1ExecutionMembersViewResponse_ExecutionMembersResponse
  operation_type: 'metadata_refresh'
  options?: Refresh_Common
  plan_id: string
  selected_member_ids?: string[]
  started_at: string
  status: 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'interrupted'
  total_components: number
  total_operations: number
  workset_id: string
}
export interface MetadataRefreshV1ExecutionMembersViewResponse_ExecutionMembersResponse {
  attention: MetadataRefreshV1ExecutionMembersViewResponse_ExecutionMemberSummaryResponse[]
  attention_total: number
  counts: MetadataRefreshV1ExecutionMembersViewResponse_ExecutionMemberCountsResponse
}
export interface MetadataRefreshV1ExecutionMembersViewResponse_ExecutionMemberSummaryResponse {
  completed_components: number
  completed_operations: number
  error_message: string
  inventory_sync_error: string
  member_id: string
  root_path: string
  status: 'pending' | 'running' | 'succeeded' | 'failed' | 'canceled'
  total_components: number
  total_operations: number
}
export interface MetadataRefreshV1ExecutionMembersViewResponse_ExecutionMemberCountsResponse {
  canceled: number
  failed: number
  pending: number
  running: number
  succeeded: number
}
export interface ExecutionProgressEvent {
  completed_components: number
  completed_operations: number
  current_component_id: string
  current_phase: string
  current_root: string
  execution_id: string
  status: 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'interrupted'
  total_components: number
  total_operations: number
}
export interface RenameV1ExecutionProgressResponse {
  completed_components: number
  completed_operations: number
  current_component_id: string
  current_phase: string
  current_root: string
  execution_id: string
  plan_id: string
  status: 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'interrupted'
  total_components: number
  total_operations: number
}
export interface MetadataRefreshV1ExecutionProgressResponse {
  completed_components: number
  completed_operations: number
  current_component_id: string
  current_phase: string
  current_root: string
  execution_id: string
  options?: Refresh_Common
  plan_id: string
  status: 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'interrupted'
  total_components: number
  total_operations: number
}
export interface ExecutionRefResponse {
  error_code?: string
  error_message?: string
  execution_id: string
  finished_at?: string
  plan_id: string
  status: 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'interrupted'
}
export interface ExecutionStartRequest {
  all_except_member_ids?: string[]
  member_ids?: string[]
}
export interface RenameV1ExecutionStartResponse {
  created: boolean
  execution: RenameV1ExecutionStartResponse_ExecutionViewResponse
}
export interface RenameV1ExecutionStartResponse_ExecutionComponentResponse {
  committed: string[]
  completed_operations: number
  component_id: string
  component_index: number
  error_code?: string
  error_message?: string
  inventory_sync_error?: string
  inventory_synced: boolean
  /**
   * @maxItems 0
   */
  kept_decisions: []
  operations: number
  partition: '' | 'matched' | 'unmatched'
  recovery: string[]
  remaining: string[]
  removed: string[]
  root_path: string
  stage?: 'precheck' | 'materialize' | 'validate' | 'commit' | 'remove' | 'skipped' | 'metadata'
  status: 'pending' | 'succeeded' | 'failed' | 'canceled'
  task_result?: Dirrename_Result
}
export interface RenameV1ExecutionStartResponse_ExecutionMembersResponse {
  attention: RenameV1ExecutionStartResponse_ExecutionMemberSummaryResponse[]
  attention_total: number
  counts: RenameV1ExecutionStartResponse_ExecutionMemberCountsResponse
}
export interface RenameV1ExecutionStartResponse_ExecutionMemberSummaryResponse {
  completed_components: number
  completed_operations: number
  error_message: string
  inventory_sync_error: string
  member_id: string
  root_path: string
  status: 'pending' | 'running' | 'succeeded' | 'failed' | 'canceled'
  total_components: number
  total_operations: number
}
export interface RenameV1ExecutionStartResponse_ExecutionMemberCountsResponse {
  canceled: number
  failed: number
  pending: number
  running: number
  succeeded: number
}
export interface MetadataRefreshV1ExecutionStartResponse {
  created: boolean
  execution: MetadataRefreshV1ExecutionStartResponse_ExecutionViewResponse
}
export interface MetadataRefreshV1ExecutionStartResponse_ExecutionMembersResponse {
  attention: MetadataRefreshV1ExecutionStartResponse_ExecutionMemberSummaryResponse[]
  attention_total: number
  counts: MetadataRefreshV1ExecutionStartResponse_ExecutionMemberCountsResponse
}
export interface MetadataRefreshV1ExecutionStartResponse_ExecutionMemberSummaryResponse {
  completed_components: number
  completed_operations: number
  error_message: string
  inventory_sync_error: string
  member_id: string
  root_path: string
  status: 'pending' | 'running' | 'succeeded' | 'failed' | 'canceled'
  total_components: number
  total_operations: number
}
export interface MetadataRefreshV1ExecutionStartResponse_ExecutionMemberCountsResponse {
  canceled: number
  failed: number
  pending: number
  running: number
  succeeded: number
}
export interface ExecutionStoppedEvent {
  execution_id: string
}
export interface ExecutionSucceededEvent {
  execution_id: string
  plan_id: string
}
export interface RenameV1ExecutionViewResponse_ExecutionComponentResponse {
  committed: string[]
  completed_operations: number
  component_id: string
  component_index: number
  error_code?: string
  error_message?: string
  inventory_sync_error?: string
  inventory_synced: boolean
  /**
   * @maxItems 0
   */
  kept_decisions: []
  operations: number
  partition: '' | 'matched' | 'unmatched'
  recovery: string[]
  remaining: string[]
  removed: string[]
  root_path: string
  stage?: 'precheck' | 'materialize' | 'validate' | 'commit' | 'remove' | 'skipped' | 'metadata'
  status: 'pending' | 'succeeded' | 'failed' | 'canceled'
  task_result?: Dirrename_Result
}
export interface RenameV1ExecutionViewResponse_ExecutionMembersResponse {
  attention: RenameV1ExecutionViewResponse_ExecutionMemberSummaryResponse[]
  attention_total: number
  counts: RenameV1ExecutionViewResponse_ExecutionMemberCountsResponse
}
export interface RenameV1ExecutionViewResponse_ExecutionMemberSummaryResponse {
  completed_components: number
  completed_operations: number
  error_message: string
  inventory_sync_error: string
  member_id: string
  root_path: string
  status: 'pending' | 'running' | 'succeeded' | 'failed' | 'canceled'
  total_components: number
  total_operations: number
}
export interface RenameV1ExecutionViewResponse_ExecutionMemberCountsResponse {
  canceled: number
  failed: number
  pending: number
  running: number
  succeeded: number
}
export interface MetadataRefreshV1ExecutionViewResponse_ExecutionMembersResponse {
  attention: MetadataRefreshV1ExecutionViewResponse_ExecutionMemberSummaryResponse[]
  attention_total: number
  counts: MetadataRefreshV1ExecutionViewResponse_ExecutionMemberCountsResponse
}
export interface MetadataRefreshV1ExecutionViewResponse_ExecutionMemberSummaryResponse {
  completed_components: number
  completed_operations: number
  error_message: string
  inventory_sync_error: string
  member_id: string
  root_path: string
  status: 'pending' | 'running' | 'succeeded' | 'failed' | 'canceled'
  total_components: number
  total_operations: number
}
export interface MetadataRefreshV1ExecutionViewResponse_ExecutionMemberCountsResponse {
  canceled: number
  failed: number
  pending: number
  running: number
  succeeded: number
}
export interface FacetResponse {
  id: string
  name: string
  work_count: number
}
export interface FileOperationItem {
  name?: string
  source: string
  target_dir?: string
}
export interface FileOperationItemResponse {
  code?: string
  message?: string
  recovered_path?: string
  source: string
  status: 'ok' | 'failed' | 'skipped' | 'not_attempted'
  target?: string
}
export interface FileOperationRefreshResponse {
  code?: string
  message?: string
  ok: boolean
}
export interface FileOperationRequest {
  items?: FileOperationRequest_FileOperationItem[]
  member_path?: string
  operation?: 'rename' | 'move' | 'soft_delete'
}
export interface FileOperationRequest_FileOperationItem {
  name?: string
  source?: string
  target_dir?: string
}
export interface FileOperationResponse {
  failed: number
  items: FileOperationResponse_FileOperationItemResponse[]
  member_path: string
  operation: 'rename' | 'move' | 'soft_delete'
  refresh: FileOperationResponse_FileOperationRefreshResponse
  succeeded: number
  untouched: number
}
export interface FileOperationResponse_FileOperationItemResponse {
  code?: string
  message?: string
  recovered_path?: string
  source: string
  status: 'ok' | 'failed' | 'skipped' | 'not_attempted'
  target?: string
}
export interface FileOperationResponse_FileOperationRefreshResponse {
  code?: string
  message?: string
  ok: boolean
}
export interface GenerationCompletedEvent {
  generation_id: string
  revision_id: string
}
export interface GenerationFailedEvent {
  error_code: string
  error_message: string
  generation_id: string
}
export interface GenerationProgressEvent {
  completed_roots: number
  current_root: string
  error_count: number
  generation_id: string
  total_roots: number
}
export interface GenerationProgressResponse {
  completed_roots: number
  current_root: string
  error_count: number
  generation_id: string
  status: 'queued' | 'running' | 'completed' | 'failed' | 'canceled' | 'interrupted'
  total_roots: number
}
export interface GenerationReplayRevisionResponse {
  created: boolean
  revision: GenerationReplayRevisionResponse_CurrentRevisionResponse | null
}
export interface GenerationReplayRevisionResponse_CurrentRevisionResponse {
  counts: GenerationReplayRevisionResponse_RevisionCountsResponse
  created_at: string
  plan_id: string
  revision_index: number
  stale: boolean | null
  status: string
  summary_reason: string
  validation_state: 'valid' | 'stale' | 'unavailable'
}
export interface GenerationReplayRevisionResponse_RevisionCountsResponse {
  blocked: number
  changed: number
  members: number
  unchanged: number
  unmet_targets: number
}
export interface GenerationStartResponse {
  created: boolean
  generation: GenerationStartResponse_GenerationViewResponse
}
export interface GenerationStartResponse_GenerationViewResponse {
  completed_roots: number
  created_at: string
  current_root: string
  error_code: string
  error_count: number
  error_message: string
  finished_at: string
  generation_id: string
  operation_type: 'conversion' | 'rename' | 'metadata_refresh'
  revision_id: string
  started_at: string
  status: 'queued' | 'running' | 'completed' | 'failed' | 'canceled' | 'interrupted'
  total_roots: number
  workset_id: string
}
export interface GenerationStoppedEvent {
  generation_id: string
}
export interface GenerationSummaryResponse {
  error_code: string
  error_message: string
  finished_at: string
  generation_id: string
  status: 'queued' | 'running' | 'completed' | 'failed' | 'canceled' | 'interrupted'
}
export interface GenerationViewResponse {
  completed_roots: number
  created_at: string
  current_root: string
  error_code: string
  error_count: number
  error_message: string
  finished_at: string
  generation_id: string
  operation_type: 'conversion' | 'rename' | 'metadata_refresh'
  revision_id: string
  started_at: string
  status: 'queued' | 'running' | 'completed' | 'failed' | 'canceled' | 'interrupted'
  total_roots: number
  workset_id: string
}
export interface HealthResponse {
  ok: boolean
  version: string
  workset_max_members: number
}
export interface IgnoredGroupResponse {
  data_group: string
  message: string
  reason: string
}
export interface ImportResponse {
  applied: ImportResponse_AppliedGroupResponse[]
  ignored: ImportResponse_IgnoredGroupResponse[]
  warnings: ImportResponse_WarningResponse[]
}
export interface ImportResponse_AppliedGroupResponse {
  data_group: string
  dl_count?: number
}
export interface ImportResponse_IgnoredGroupResponse {
  data_group: string
  message: string
  reason: string
}
export interface ImportResponse_WarningResponse {
  code: string
  message: string
  work_id?: string
}
export interface LibraryCreateRequest {
  name?: string
  root_path?: string
}
export interface LibraryListResponse {
  libraries: LibraryListResponse_LibraryResponse[]
}
export interface LibraryListResponse_LibraryResponse {
  created_at: string
  id: string
  last_scan_at: string | null
  last_scan_error: string
  last_scan_status: string
  name: string
  root_path: string
  storage_class: 'hdd' | 'ssd'
  updated_at: string
}
export interface LibraryPatchRequest {
  name?: string | null
  storage_class?: 'hdd' | 'ssd'
}
export interface LibraryRefResponse {
  library_id: string
  name: string
  root_path: string
}
export interface LibraryResponse {
  created_at: string
  id: string
  last_scan_at: string | null
  last_scan_error: string
  last_scan_status: string
  name: string
  root_path: string
  storage_class: 'hdd' | 'ssd'
  updated_at: string
}
export interface ListeningResponse {
  finished_at: string | null
  position_sec: number | null
  rendition: string
  track_count: number | null
  track_index: number | null
  track_key: string | null
  updated_at: string
}
export interface ListeningSummaryResponse {
  state: 'none' | 'listening' | 'finished'
  track_count: number | null
  track_index: number | null
  updated_at: string | null
}
export interface LocalCoverResponse {
  copy_id: string
  rel_path: string
  thumb_v: string
  version: string
}
export interface MediaFactsLibraryStat {
  library_id: string
  pending: number
  unmeasurable: number
}
export interface MediaFactsStatus {
  libraries: MediaFactsStatus_MediaFactsLibraryStat[]
  state: 'running' | 'paused' | 'idle'
}
export interface MediaFactsStatus_MediaFactsLibraryStat {
  library_id: string
  pending: number
  unmeasurable: number
}
export interface RenameV1MemberConclusionResponse {
  category?: string
  excluded: boolean
  has_components: boolean
  input: 'none' | 'valid' | 'stale' | 'missing' | 'unavailable'
  member_id: string
  task?: Dirrename_Outcome
}
export interface MetadataRefreshV1MemberConclusionResponse {
  category?: string
  excluded: boolean
  has_components: boolean
  input: 'none' | 'valid' | 'stale' | 'missing' | 'unavailable'
  member_id: string
  task?: Refresh_Outcome
}
export interface MemberResponse {
  dir_id: string
  folder_name: string
  folder_path: string
  member_id: string
  rel_path: string
}
export interface RenameV1MemberReviewResponse {
  conclusion: RenameV1MemberReviewResponse_MemberConclusionResponse
  effective: Dirrename_Common | null
  excluded: boolean
  folder_path: string
  member_id: string
  member_name: string
  plan_id: string
  roots: RenameV1MemberReviewResponse_RootValidationResponse[]
  sources: {
    [k: string]: string
  } | null
  task: RenameV1MemberReviewResponse_TaskEnvelopeResponse
}
export interface RenameV1MemberReviewResponse_MemberConclusionResponse {
  category?: string
  excluded: boolean
  has_components: boolean
  input: 'none' | 'valid' | 'stale' | 'missing' | 'unavailable'
  member_id: string
  task?: Dirrename_Outcome
}
export interface RenameV1MemberReviewResponse_RootValidationResponse {
  entry_count: number
  inventory_fingerprint: string
  root_error_code: string
  root_error_message: string
  root_index: number
  root_path: string
  root_status: string
  stale: boolean
}
export interface RenameV1MemberReviewResponse_TaskEnvelopeResponse {
  kind: 'rename'
  payload: RenameV1MemberReviewResponse_TaskPayloadResponse
  schema_version: 1
}
export interface RenameV1MemberReviewResponse_TaskPayloadResponse {
  classifier: TaskClassifierSnapshot
  components: Dirrename_Outcome[]
  policy: Dirrename_Common
  policy_hash: string
  summary: Workset_PlanSummary
}
export interface Workset_PlanSummary {
  blocked_count: number
  component_count: number
  error_count: number
  operation_count: number
  summary_reason: string
  unmet_targets?: number
}
export interface MetadataRefreshV1MemberReviewResponse {
  conclusion: MetadataRefreshV1MemberReviewResponse_MemberConclusionResponse
  effective: Refresh_Common | null
  excluded: boolean
  folder_path: string
  member_id: string
  member_name: string
  plan_id: string
  roots: MetadataRefreshV1MemberReviewResponse_RootValidationResponse[]
  sources: {
    [k: string]: string
  } | null
  task: MetadataRefreshV1MemberReviewResponse_TaskEnvelopeResponse
}
export interface MetadataRefreshV1MemberReviewResponse_MemberConclusionResponse {
  category?: string
  excluded: boolean
  has_components: boolean
  input: 'none' | 'valid' | 'stale' | 'missing' | 'unavailable'
  member_id: string
  task?: Refresh_Outcome
}
export interface MetadataRefreshV1MemberReviewResponse_RootValidationResponse {
  entry_count: number
  inventory_fingerprint: string
  root_error_code: string
  root_error_message: string
  root_index: number
  root_path: string
  root_status: string
  stale: boolean
}
export interface MetadataRefreshV1MemberReviewResponse_TaskEnvelopeResponse {
  kind: 'metadata_refresh'
  payload: MetadataRefreshV1MemberReviewResponse_TaskPayloadResponse
  schema_version: 1
}
export interface MetadataRefreshV1MemberReviewResponse_TaskPayloadResponse {
  classifier: TaskClassifierSnapshot
  components: Refresh_Outcome[]
  policy: Refresh_Common
  policy_hash: string
  summary: Refresh_PlanSummary
}
export interface Refresh_PlanSummary {
  blocked_count: number
  component_count: number
  error_count: number
  operation_count: number
  summary_reason: string
  unmet_targets?: number
  work_count: number
}
export interface MemberTreeNodeResponse {
  bitrate: number | null
  children?: MemberTreeNodeResponse_MemberTreeNodeResponse[]
  format: string
  name: string
  path: string
  rel_path: string
  size?: number
  type: 'dir' | 'file'
}
export interface MemberTreeNodeResponse_MemberTreeNodeResponse {
  bitrate: number | null
  children?: MemberTreeNodeResponse_MemberTreeNodeResponse[]
  format: string
  name: string
  path: string
  rel_path: string
  size?: number
  type: 'dir' | 'file'
}
export interface MetadataRefreshV1OperationResponse {
  active_execution: MetadataRefreshV1OperationResponse_ExecutionProgressResponse | null
  active_generation: MetadataRefreshV1OperationResponse_GenerationProgressResponse | null
  current_revision: MetadataRefreshV1OperationResponse_CurrentRevisionResponse | null
  latest_execution: MetadataRefreshV1OperationResponse_ExecutionRefResponse | null
  latest_generation: MetadataRefreshV1OperationResponse_GenerationSummaryResponse | null
  max_participants?: number
  operation_type: 'conversion' | 'rename' | 'metadata_refresh'
  planning_state: 'unplanned' | 'planned' | 'needs_planning' | 'planning' | 'orphaned'
  version: number
  workset_id: string
}
export interface MetadataRefreshV1OperationResponse_ExecutionProgressResponse {
  completed_components: number
  completed_operations: number
  current_component_id: string
  current_phase: string
  current_root: string
  execution_id: string
  options?: Refresh_Common
  plan_id: string
  status: 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'interrupted'
  total_components: number
  total_operations: number
}
export interface MetadataRefreshV1OperationResponse_GenerationProgressResponse {
  completed_roots: number
  current_root: string
  error_count: number
  generation_id: string
  status: 'queued' | 'running' | 'completed' | 'failed' | 'canceled' | 'interrupted'
  total_roots: number
}
export interface MetadataRefreshV1OperationResponse_CurrentRevisionResponse {
  counts: MetadataRefreshV1OperationResponse_RevisionCountsResponse
  created_at: string
  plan_id: string
  revision_index: number
  stale: boolean | null
  status: string
  summary_reason: string
  validation_state: 'valid' | 'stale' | 'unavailable'
}
export interface MetadataRefreshV1OperationResponse_RevisionCountsResponse {
  blocked: number
  changed: number
  members: number
  unchanged: number
  unmet_targets: number
}
export interface MetadataRefreshV1OperationResponse_ExecutionRefResponse {
  error_code?: string
  error_message?: string
  execution_id: string
  finished_at?: string
  plan_id: string
  status: 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'interrupted'
}
export interface MetadataRefreshV1OperationResponse_GenerationSummaryResponse {
  error_code: string
  error_message: string
  finished_at: string
  generation_id: string
  status: 'queued' | 'running' | 'completed' | 'failed' | 'canceled' | 'interrupted'
}
export interface MetadataRefreshV1RevisionMemberResponse {
  conclusion: MetadataRefreshV1RevisionMemberResponse_MemberConclusionResponse
  effective: Refresh_Common | null
  excluded: boolean
  folder_path: string
  member_id: string
  member_name: string
  sources: {
    [k: string]: string
  } | null
}
export interface MetadataRefreshV1RevisionMemberResponse_MemberConclusionResponse {
  category?: string
  excluded: boolean
  has_components: boolean
  input: 'none' | 'valid' | 'stale' | 'missing' | 'unavailable'
  member_id: string
  task?: Refresh_Outcome
}
export interface MetadataRefreshV1RevisionResponse {
  category_counts: {
    [k: string]: number
  } | null
  category_total?: number
  counts: MetadataRefreshV1RevisionResponse_RevisionCountsResponse
  created_at: string
  execution: MetadataRefreshV1RevisionResponse_ExecutionRefResponse | null
  members: MetadataRefreshV1RevisionResponse_MemberConclusionResponse[]
  plan_id: string
  revision_index: number
  root_path: string
  snapshot_token: string
  status: string
  summary: MetadataRefreshV1RevisionResponse_PlanSummaryResponse
  task: MetadataRefreshV1RevisionResponse_TaskEnvelopeResponse
}
export interface MetadataRefreshV1RevisionResponse_RevisionCountsResponse {
  blocked: number
  changed: number
  members: number
  unchanged: number
  unmet_targets: number
}
export interface MetadataRefreshV1RevisionResponse_ExecutionRefResponse {
  error_code?: string
  error_message?: string
  execution_id: string
  finished_at?: string
  plan_id: string
  status: 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'interrupted'
}
export interface MetadataRefreshV1RevisionResponse_MemberConclusionResponse {
  category?: string
  excluded: boolean
  has_components: boolean
  input: 'none' | 'valid' | 'stale' | 'missing' | 'unavailable'
  member_id: string
  task?: Refresh_Outcome
}
export interface MetadataRefreshV1RevisionResponse_PlanSummaryResponse {
  actionable_count: number
  error_count: number
  operation_count: number
  summary_reason: string
  total_count: number
}
export interface MetadataRefreshV1RevisionResponse_TaskEnvelopeResponse {
  kind: 'metadata_refresh'
  payload: MetadataRefreshV1RevisionResponse_TaskPayloadResponse
  schema_version: 1
}
export interface MetadataRefreshV1RevisionResponse_TaskPayloadResponse {
  classifier: TaskClassifierSnapshot
  policy: Refresh_Common
  policy_hash: string
  summary: Refresh_PlanSummary
}
export interface MetadataRefreshV1TaskDraftEnvelope {
  kind: 'metadata_refresh'
  payload: Refresh_Draft
  schema_version: 1
}
export interface MetadataRefreshV1TaskEnvelopeResponse {
  kind: 'metadata_refresh'
  payload: MetadataRefreshV1TaskEnvelopeResponse_TaskPayloadResponse
  schema_version: 1
}
export interface MetadataRefreshV1TaskEnvelopeResponse_TaskPayloadResponse {
  classifier: TaskClassifierSnapshot
  components?: Refresh_Outcome[]
  policy: Refresh_Common
  policy_hash: string
  summary: Refresh_PlanSummary
}
export interface MetadataRefreshV1TaskPayloadResponse {
  classifier: TaskClassifierSnapshot
  components?: Refresh_Outcome[]
  policy: Refresh_Common
  policy_hash: string
  summary: Refresh_PlanSummary
}
export interface MetadataRefreshV1TaskSettingsRequest {
  settings?: Refresh_CommonRequest
}
export interface Refresh_CommonRequest {
  download_cover?: boolean
  locale?: 'ja-jp' | 'zh-cn' | 'zh-tw'
  mode?: 'fill_missing' | 'refresh_dynamic' | 'refresh_all'
}
export interface MetadataRefreshV1TaskSettingsResponse {
  operation_type: 'metadata_refresh'
  settings: Refresh_Common
}
export interface RenameV1OperationResponse {
  active_execution: RenameV1OperationResponse_ExecutionProgressResponse | null
  active_generation: RenameV1OperationResponse_GenerationProgressResponse | null
  current_revision: RenameV1OperationResponse_CurrentRevisionResponse | null
  latest_execution: RenameV1OperationResponse_ExecutionRefResponse | null
  latest_generation: RenameV1OperationResponse_GenerationSummaryResponse | null
  max_participants?: number
  operation_type: 'conversion' | 'rename' | 'metadata_refresh'
  planning_state: 'unplanned' | 'planned' | 'needs_planning' | 'planning' | 'orphaned'
  version: number
  workset_id: string
}
export interface RenameV1OperationResponse_ExecutionProgressResponse {
  completed_components: number
  completed_operations: number
  current_component_id: string
  current_phase: string
  current_root: string
  execution_id: string
  plan_id: string
  status: 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'interrupted'
  total_components: number
  total_operations: number
}
export interface RenameV1OperationResponse_GenerationProgressResponse {
  completed_roots: number
  current_root: string
  error_count: number
  generation_id: string
  status: 'queued' | 'running' | 'completed' | 'failed' | 'canceled' | 'interrupted'
  total_roots: number
}
export interface RenameV1OperationResponse_CurrentRevisionResponse {
  counts: RenameV1OperationResponse_RevisionCountsResponse
  created_at: string
  plan_id: string
  revision_index: number
  stale: boolean | null
  status: string
  summary_reason: string
  validation_state: 'valid' | 'stale' | 'unavailable'
}
export interface RenameV1OperationResponse_RevisionCountsResponse {
  blocked: number
  changed: number
  members: number
  unchanged: number
  unmet_targets: number
}
export interface RenameV1OperationResponse_ExecutionRefResponse {
  error_code?: string
  error_message?: string
  execution_id: string
  finished_at?: string
  plan_id: string
  status: 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'interrupted'
}
export interface RenameV1OperationResponse_GenerationSummaryResponse {
  error_code: string
  error_message: string
  finished_at: string
  generation_id: string
  status: 'queued' | 'running' | 'completed' | 'failed' | 'canceled' | 'interrupted'
}
export interface OutboundProxyPutRequest {
  mode?: 'system' | 'manual' | 'direct'
  url?: string | null
}
export interface OutboundProxyResponse {
  mode: 'system' | 'manual' | 'direct'
  url: string
}
export interface PlanSummaryResponse {
  actionable_count: number
  error_count: number
  operation_count: number
  summary_reason: string
  total_count: number
}
export interface PreferencesResponse {
  default_library_id?: string
  track_formats: string[]
}
export interface PreferencesUpdateRequest {
  default_library_id?: string | null
  track_formats?: string[] | null
}
export interface RandomWorkResponse {
  index: number
  rj: string
}
export interface Reconcile_VariantDecision {
  decisions: Reconcile_VariantDecision_Reconcile_FileDecision[]
  stem: string
}
export interface Reconcile_VariantDecision_Reconcile_FileDecision {
  message?: string
  path: string
  reason_code?: string
  resolution: 'keep' | 'delete' | 'encode'
  target_path?: string
}
export interface RecordCreateRequest {
  expected_current_id?: string
  selection?: RecordCreateRequest_SelectionRequest
  title?: string
}
export interface RecordCreateRequest_SelectionRequest {
  all?: boolean
  dir_ids?: string[]
  except_dir_ids?: string[]
}
export interface RecordCreateResponse {
  created: boolean
  recorded: number
  skipped: RecordCreateResponse_SkippedFolderResponse[]
  workset: RecordCreateResponse_WorksetResponse
}
export interface RecordCreateResponse_SkippedFolderResponse {
  dir_id: string
  reason: 'unknown_dir' | 'duplicate' | 'ambiguous'
}
export interface RecordCreateResponse_WorksetResponse {
  created_at: string
  library: RecordCreateResponse_LibraryRefResponse | null
  members: RecordCreateResponse_MemberResponse[]
  title: string
  updated_at: string
  version: number
  workset_id: string
}
export interface RecordCreateResponse_LibraryRefResponse {
  library_id: string
  name: string
  root_path: string
}
export interface RecordCreateResponse_MemberResponse {
  dir_id: string
  folder_name: string
  folder_path: string
  member_id: string
  rel_path: string
}
export interface Refresh_DraftRequest {
  common?: Refresh_DraftRequest_Refresh_Common
  members?: Refresh_DraftRequest_Refresh_Member[] | null
  schema_version?: number
}
export interface Refresh_DraftRequest_Refresh_Common {
  download_cover?: boolean
  locale?: 'ja-jp' | 'zh-cn' | 'zh-tw'
  mode?: 'fill_missing' | 'refresh_dynamic' | 'refresh_all'
}
export interface Refresh_DraftRequest_Refresh_Member {
  excluded?: boolean
  member_id?: string
}
export interface RelationResponse {
  language: string | null
  original_rj: string | null
  parent_rj: string | null
  role: ('original' | 'parent' | 'child') | null
}
export interface RelationWarningResponse {
  code: string
  message: string
  rj: string
}
export interface RenameV1RevisionMemberResponse {
  conclusion: RenameV1RevisionMemberResponse_MemberConclusionResponse
  effective: Dirrename_Common | null
  excluded: boolean
  folder_path: string
  member_id: string
  member_name: string
  sources: {
    [k: string]: string
  } | null
}
export interface RenameV1RevisionMemberResponse_MemberConclusionResponse {
  category?: string
  excluded: boolean
  has_components: boolean
  input: 'none' | 'valid' | 'stale' | 'missing' | 'unavailable'
  member_id: string
  task?: Dirrename_Outcome
}
export interface RenameV1RevisionResponse {
  category_counts: {
    [k: string]: number
  } | null
  category_total?: number
  counts: RenameV1RevisionResponse_RevisionCountsResponse
  created_at: string
  execution: RenameV1RevisionResponse_ExecutionRefResponse | null
  members: RenameV1RevisionResponse_MemberConclusionResponse[]
  plan_id: string
  revision_index: number
  root_path: string
  snapshot_token: string
  status: string
  summary: RenameV1RevisionResponse_PlanSummaryResponse
  task: RenameV1RevisionResponse_TaskEnvelopeResponse
}
export interface RenameV1RevisionResponse_RevisionCountsResponse {
  blocked: number
  changed: number
  members: number
  unchanged: number
  unmet_targets: number
}
export interface RenameV1RevisionResponse_ExecutionRefResponse {
  error_code?: string
  error_message?: string
  execution_id: string
  finished_at?: string
  plan_id: string
  status: 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'interrupted'
}
export interface RenameV1RevisionResponse_MemberConclusionResponse {
  category?: string
  excluded: boolean
  has_components: boolean
  input: 'none' | 'valid' | 'stale' | 'missing' | 'unavailable'
  member_id: string
  task?: Dirrename_Outcome
}
export interface RenameV1RevisionResponse_PlanSummaryResponse {
  actionable_count: number
  error_count: number
  operation_count: number
  summary_reason: string
  total_count: number
}
export interface RenameV1RevisionResponse_TaskEnvelopeResponse {
  kind: 'rename'
  payload: RenameV1RevisionResponse_TaskPayloadResponse
  schema_version: 1
}
export interface RenameV1RevisionResponse_TaskPayloadResponse {
  classifier: TaskClassifierSnapshot
  policy: Dirrename_Common
  policy_hash: string
  summary: Workset_PlanSummary
}
export interface RenameV1TaskDraftEnvelope {
  kind: 'rename'
  payload: Dirrename_Draft
  schema_version: 1
}
export interface RenameV1TaskEnvelopeResponse {
  kind: 'rename'
  payload: RenameV1TaskEnvelopeResponse_TaskPayloadResponse
  schema_version: 1
}
export interface RenameV1TaskEnvelopeResponse_TaskPayloadResponse {
  classifier: TaskClassifierSnapshot
  components?: Dirrename_Outcome[]
  policy: Dirrename_Common
  policy_hash: string
  summary: Workset_PlanSummary
}
export interface RenameV1TaskPayloadResponse {
  classifier: TaskClassifierSnapshot
  components?: Dirrename_Outcome[]
  policy: Dirrename_Common
  policy_hash: string
  summary: Workset_PlanSummary
}
export interface RenameV1TaskSettingsRequest {
  settings?: Dirrename_CommonRequest
}
export interface RenameV1TaskSettingsResponse {
  operation_type: 'rename'
  settings: Dirrename_Common
}
export interface RevisionCountsResponse {
  blocked: number
  changed: number
  members: number
  unchanged: number
  unmet_targets: number
}
export interface RootValidationResponse {
  entry_count: number
  inventory_fingerprint: string
  root_error_code: string
  root_error_message: string
  root_index: number
  root_path: string
  root_status: string
  stale: boolean
}
export interface ScanEventData {
  code?: string
  dirs_scanned?: number
  dirs_with_multiple_numbers?: number
  example_dirs?: string[]
  examples_truncated?: boolean
  files_scanned?: number
  message?: string
  root_path?: string
  scan_id?: string
  stage: string
  unaddressable_names?: number
}
export interface ScanRequest {
  mode?: 'full'
  root_path?: string | null
}
export interface SelectionRequest {
  all?: boolean
  dir_ids?: string[]
  except_dir_ids?: string[]
}
export interface SessionCreateRequest {
  token?: string | null
}
export interface SessionStatusEvent {
  status: string
}
export interface SkippedFolderResponse {
  dir_id: string
  reason: 'unknown_dir' | 'duplicate' | 'ambiguous'
}
export interface SourceEntityResponse {
  id: string
  name: string
  source: string
  source_id: string | null
}
export interface TagFacetsResponse {
  source_tags: TagFacetsResponse_FacetResponse[]
  user_tags: TagFacetsResponse_FacetResponse[]
}
export interface TagFacetsResponse_FacetResponse {
  id: string
  name: string
  work_count: number
}
export interface TreeNodeResponse {
  held: boolean
  is_work: boolean
  language: string | null
  relation: TreeNodeResponse_RelationResponse
  rj: string
  role: '' | 'original' | 'parent' | 'child' | 'edition_page' | 'translation'
  source_url: string | null
}
export interface TreeNodeResponse_RelationResponse {
  language: string | null
  original_rj: string | null
  parent_rj: string | null
  role: ('original' | 'parent' | 'child') | null
}
export interface TreeResponse {
  dir_id: string
  member_path: string
  refreshed?: boolean
  rj: string | null
  tree: TreeResponse_MemberTreeNodeResponse | null
}
export interface TreeResponse_MemberTreeNodeResponse {
  bitrate: number | null
  children?: TreeResponse_MemberTreeNodeResponse[]
  format: string
  name: string
  path: string
  rel_path: string
  size?: number
  type: 'dir' | 'file'
}
export interface UpdateCheckResponse {
  latest_version?: string
  release_url?: string
  state: 'checking' | 'none' | 'available'
}
export interface UserDataResponse {
  favorite: boolean
  note: string
  rating: number | null
}
export interface UserTagListResponse {
  user_tags: UserTagListResponse_UserTagResponse[]
}
export interface UserTagListResponse_UserTagResponse {
  id: string
  name: string
}
export interface UserTagRequest {
  name?: string
}
export interface UserTagResponse {
  id: string
  name: string
}
export interface VoiceActorFacetsResponse {
  voice_actors: VoiceActorFacetsResponse_FacetResponse[]
}
export interface VoiceActorFacetsResponse_FacetResponse {
  id: string
  name: string
  work_count: number
}
export interface WarningResponse {
  code: string
  message: string
  work_id?: string
}
export interface WorkContentCopyResponse {
  copy_id: string
  dir_id: string
  library_id: string
  online: boolean
  tree?: WorkContentCopyResponse_WorkContentNodeResponse
}
export interface WorkContentDirNodeJSON {
  children: WorkContentDirNodeJSON_WorkContentNodeResponse[]
  name: string
  rel_path: string
}
export interface WorkContentDirNodeJSON1 {
  children: WorkContentDirNodeJSON_WorkContentNodeResponse[]
  name: string
  rel_path: string
}
export interface WorkContentFileNodeJSON {
  bitrate: number | null
  duration_sec: number | null
  format: string | null
  height: number | null
  kind: 'aud' | 'img' | 'txt' | 'lrc' | 'vid' | 'bin'
  name: string
  rel_path: string
  size: number
  thumb_v: string | null
  width: number | null
}
export interface WorkContentDirNodeJSON2 {
  children: WorkContentDirNodeJSON_WorkContentNodeResponse[]
  name: string
  rel_path: string
}
export interface WorkContentResponse {
  copies: WorkContentResponse_WorkContentCopyResponse[]
  rj: string
}
export interface WorkContentResponse_WorkContentCopyResponse {
  copy_id: string
  dir_id: string
  library_id: string
  online: boolean
  tree?: WorkContentResponse_WorkContentNodeResponse
}
export interface WorkDetailResponse {
  age_category: number | null
  circle: string | null
  collected_at: string
  collection_ids: string[]
  copies: WorkDetailResponse_CopyResponse[]
  copy_count: number
  cover_url: string | null
  dl_count: number | null
  edition_tree: WorkDetailResponse_EditionSlotResponse[]
  favorite: boolean
  has_note: boolean
  language: string | null
  language_editions: WorkDetailResponse_EditionResponse[]
  listening: WorkDetailResponse_ListeningResponse[]
  local_cover: WorkDetailResponse_LocalCoverResponse | null
  note: string
  nsfw: boolean | null
  rating: number | null
  relation: WorkDetailResponse_RelationResponse
  relation_nodes: WorkDetailResponse_TreeNodeResponse[]
  relation_warnings: WorkDetailResponse_RelationWarningResponse[]
  release_date: string | null
  rj: string
  sample_cover_url: string | null
  series_id: string | null
  series_name: string | null
  source_tags: WorkDetailResponse_SourceEntityResponse[]
  source_url: string | null
  title: string | null
  user_tags: WorkDetailResponse_UserTagResponse[]
  voice_actors: WorkDetailResponse_SourceEntityResponse[]
}
export interface WorkDetailResponse_CopyResponse {
  copy_id: string
  dir_id: string
  library_id: string
  rel_path: string
}
export interface WorkDetailResponse_EditionSlotResponse {
  held: boolean
  label: string | null
  language: string
  root: WorkDetailResponse_TreeNodeResponse
  translations: WorkDetailResponse_TreeNodeResponse[]
}
export interface WorkDetailResponse_TreeNodeResponse {
  held: boolean
  is_work: boolean
  language: string | null
  relation: WorkDetailResponse_RelationResponse
  rj: string
  role: '' | 'original' | 'parent' | 'child' | 'edition_page' | 'translation'
  source_url: string | null
}
export interface WorkDetailResponse_RelationResponse {
  language: string | null
  original_rj: string | null
  parent_rj: string | null
  role: ('original' | 'parent' | 'child') | null
}
export interface WorkDetailResponse_EditionResponse {
  display_order: number
  edition_id: string | null
  label: string | null
  language: string
  version_root_rj: string
}
export interface WorkDetailResponse_ListeningResponse {
  finished_at: string | null
  position_sec: number | null
  rendition: string
  track_count: number | null
  track_index: number | null
  track_key: string | null
  updated_at: string
}
export interface WorkDetailResponse_LocalCoverResponse {
  copy_id: string
  rel_path: string
  thumb_v: string
  version: string
}
export interface WorkDetailResponse_RelationWarningResponse {
  code: string
  message: string
  rj: string
}
export interface WorkDetailResponse_SourceEntityResponse {
  id: string
  name: string
  source: string
  source_id: string | null
}
export interface WorkDetailResponse_UserTagResponse {
  id: string
  name: string
}
export interface WorkImportCircleRequest {
  name?: string
  source_circle_id?: string | null
}
export interface WorkImportCoreRequest {
  age_category?: number | null
  circle?: WorkImportCoreRequest_WorkImportCircleRequest | null
  cover_url?: string | null
  fetched_at?: string
  language?: string | null
  nsfw?: boolean | null
  release_date?: string | null
  sample_cover_url?: string | null
  series_id?: string | null
  series_name?: string | null
  source?: string
  source_url?: string | null
  tags?: WorkImportCoreRequest_WorkImportTagRequest[]
  title?: string | null
  voice_actors?: WorkImportCoreRequest_WorkImportVoiceActorRequest[]
}
export interface WorkImportCoreRequest_WorkImportCircleRequest {
  name?: string
  source_circle_id?: string | null
}
export interface WorkImportCoreRequest_WorkImportTagRequest {
  name?: string
  source_tag_id?: string | null
}
export interface WorkImportCoreRequest_WorkImportVoiceActorRequest {
  name?: string
  source_voice_actor_id?: string | null
}
export interface WorkImportEditionItemRequest {
  display_order?: number
  label?: string | null
  language?: string
  version_root_rj?: string
}
export interface WorkImportEditionsRequest {
  edition_id?: string | null
  fetched_at?: string
  items?: WorkImportEditionsRequest_WorkImportEditionItemRequest[]
  source?: string
}
export interface WorkImportEditionsRequest_WorkImportEditionItemRequest {
  display_order?: number
  label?: string | null
  language?: string
  version_root_rj?: string
}
export interface WorkImportRelationsRequest {
  child_rjs?: string[]
  fetched_at?: string
  language?: string | null
  original_rj?: string | null
  parent_rj?: string | null
  role?: string
  source?: string
}
export interface WorkImportStatisticsRequest {
  dl_count?: number | null
  fetched_at?: string
}
export interface WorkImportTagRequest {
  name?: string
  source_tag_id?: string | null
}
export interface WorkImportVoiceActorRequest {
  name?: string
  source_voice_actor_id?: string | null
}
export interface WorkListResponse {
  total: number
  works: WorkListResponse_WorkSummaryResponse[]
}
export interface WorkListResponse_WorkSummaryResponse {
  age_category: number | null
  circle: string | null
  collected_at: string
  copy_count: number
  cover_url: string | null
  dl_count: number | null
  favorite: boolean
  has_note: boolean
  language: string | null
  listening: WorkListResponse_ListeningSummaryResponse
  local_cover: WorkListResponse_LocalCoverResponse | null
  nsfw: boolean | null
  rating: number | null
  release_date: string | null
  rj: string
  title: string | null
  user_tags: WorkListResponse_UserTagResponse[]
}
export interface WorkListResponse_ListeningSummaryResponse {
  state: 'none' | 'listening' | 'finished'
  track_count: number | null
  track_index: number | null
  updated_at: string | null
}
export interface WorkListResponse_LocalCoverResponse {
  copy_id: string
  rel_path: string
  thumb_v: string
  version: string
}
export interface WorkListResponse_UserTagResponse {
  id: string
  name: string
}
export interface WorkListeningRequest {
  client?: string
  finished?: boolean
  position_sec?: number
  rendition?: string
  seq?: number
  track_count?: number
  track_index?: number
  track_key?: string
}
export interface WorkMetadataImportRequest {
  core?: WorkMetadataImportRequest_WorkImportCoreRequest | null
  editions?: WorkMetadataImportRequest_WorkImportEditionsRequest | null
  relations?: WorkMetadataImportRequest_WorkImportRelationsRequest | null
  statistics?: WorkMetadataImportRequest_WorkImportStatisticsRequest | null
}
export interface WorkMetadataImportRequest_WorkImportCoreRequest {
  age_category?: number | null
  circle?: WorkMetadataImportRequest_WorkImportCircleRequest | null
  cover_url?: string | null
  fetched_at?: string
  language?: string | null
  nsfw?: boolean | null
  release_date?: string | null
  sample_cover_url?: string | null
  series_id?: string | null
  series_name?: string | null
  source?: string
  source_url?: string | null
  tags?: WorkMetadataImportRequest_WorkImportTagRequest[]
  title?: string | null
  voice_actors?: WorkMetadataImportRequest_WorkImportVoiceActorRequest[]
}
export interface WorkMetadataImportRequest_WorkImportCircleRequest {
  name?: string
  source_circle_id?: string | null
}
export interface WorkMetadataImportRequest_WorkImportTagRequest {
  name?: string
  source_tag_id?: string | null
}
export interface WorkMetadataImportRequest_WorkImportVoiceActorRequest {
  name?: string
  source_voice_actor_id?: string | null
}
export interface WorkMetadataImportRequest_WorkImportEditionsRequest {
  edition_id?: string | null
  fetched_at?: string
  items?: WorkMetadataImportRequest_WorkImportEditionItemRequest[]
  source?: string
}
export interface WorkMetadataImportRequest_WorkImportEditionItemRequest {
  display_order?: number
  label?: string | null
  language?: string
  version_root_rj?: string
}
export interface WorkMetadataImportRequest_WorkImportRelationsRequest {
  child_rjs?: string[]
  fetched_at?: string
  language?: string | null
  original_rj?: string | null
  parent_rj?: string | null
  role?: string
  source?: string
}
export interface WorkMetadataImportRequest_WorkImportStatisticsRequest {
  dl_count?: number | null
  fetched_at?: string
}
export interface WorkSummaryResponse {
  age_category: number | null
  circle: string | null
  collected_at: string
  copy_count: number
  cover_url: string | null
  dl_count: number | null
  favorite: boolean
  has_note: boolean
  language: string | null
  listening: WorkSummaryResponse_ListeningSummaryResponse
  local_cover: WorkSummaryResponse_LocalCoverResponse | null
  nsfw: boolean | null
  rating: number | null
  release_date: string | null
  rj: string
  title: string | null
  user_tags: WorkSummaryResponse_UserTagResponse[]
}
export interface WorkSummaryResponse_ListeningSummaryResponse {
  state: 'none' | 'listening' | 'finished'
  track_count: number | null
  track_index: number | null
  updated_at: string | null
}
export interface WorkSummaryResponse_LocalCoverResponse {
  copy_id: string
  rel_path: string
  thumb_v: string
  version: string
}
export interface WorkSummaryResponse_UserTagResponse {
  id: string
  name: string
}
export interface WorkUserDataPatchRequest {
  favorite?: boolean | null
  note?: string | null
  rating?: number | null
}
export interface WorkUserTagsRequest {
  tag_ids?: string[]
}
export interface WorksetListResponse {
  next_cursor?: string
  worksets: WorksetListResponse_WorksetResponse[]
}
export interface WorksetListResponse_WorksetResponse {
  created_at: string
  library: WorksetListResponse_LibraryRefResponse | null
  members: WorksetListResponse_MemberResponse[]
  title: string
  updated_at: string
  version: number
  workset_id: string
}
export interface WorksetListResponse_LibraryRefResponse {
  library_id: string
  name: string
  root_path: string
}
export interface WorksetListResponse_MemberResponse {
  dir_id: string
  folder_name: string
  folder_path: string
  member_id: string
  rel_path: string
}
export interface WorksetPatchRequest {
  title?: string
}
export interface WorksetResponse {
  created_at: string
  library: WorksetResponse_LibraryRefResponse | null
  members: WorksetResponse_MemberResponse[]
  title: string
  updated_at: string
  version: number
  workset_id: string
}
export interface WorksetResponse_LibraryRefResponse {
  library_id: string
  name: string
  root_path: string
}
export interface WorksetResponse_MemberResponse {
  dir_id: string
  folder_name: string
  folder_path: string
  member_id: string
  rel_path: string
}

export type WireExecutionEvent = { type: 'canceled'; data: WireSchemas['executionStoppedEvent'] } | { type: 'component'; data: WireSchemas['executionComponentResponse'] } | { type: 'error'; data: WireSchemas['sseError'] } | { type: 'execution_snapshot'; data: WireSchemas['executionViewResponse'] } | { type: 'failed'; data: WireSchemas['executionFailedEvent'] } | { type: 'interrupted'; data: WireSchemas['executionStoppedEvent'] } | { type: 'member'; data: WireSchemas['executionMemberEvent'] } | { type: 'message'; data: WireSchemas['sessionStatusEvent'] } | { type: 'progress'; data: WireSchemas['executionProgressEvent'] } | { type: 'succeeded'; data: WireSchemas['executionSucceededEvent'] }

export type WireGenerationEvent = { type: 'canceled'; data: WireSchemas['generationStoppedEvent'] } | { type: 'completed'; data: WireSchemas['generationCompletedEvent'] } | { type: 'error'; data: WireSchemas['sseError'] } | { type: 'failed'; data: WireSchemas['generationFailedEvent'] } | { type: 'interrupted'; data: WireSchemas['generationStoppedEvent'] } | { type: 'message'; data: WireSchemas['sessionStatusEvent'] } | { type: 'progress'; data: WireSchemas['generationProgressEvent'] } | { type: 'session_snapshot'; data: WireSchemas['generationViewResponse'] }

export type WireMediaEvent = { type: 'changed'; data: WireSchemas['emptyEvent'] } | { type: 'status'; data: WireSchemas['mediaFactsStatus'] } | { type: 'works_changed'; data: WireSchemas['emptyEvent'] }

export type WireScanEvent = { type: 'cancelled'; data: WireSchemas['scanEventData'] } | { type: 'completed'; data: WireSchemas['scanEventData'] } | { type: 'error'; data: WireSchemas['scanEventData'] } | { type: 'progress'; data: WireSchemas['scanEventData'] } | { type: 'started'; data: WireSchemas['scanEventData'] }
