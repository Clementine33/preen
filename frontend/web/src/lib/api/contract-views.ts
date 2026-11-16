import type { SchemaId } from './contracts.gen'
import type * as Views from './views'
import type { WireSchemas } from './wire.gen'

// Wire bytes use generated types. These refinements describe the client's
// existing presentation vocabulary; field ownership stays with each wire root.
interface ViewBindings {
  updateCheckResponse: Views.UpdateCheck
  cleanupPreviewResponse: Views.CleanupPreview
  cleanupRunResponse: Views.CleanupRun
  collectionDetailResponse: Views.CollectionDetail
  conversion_DraftDoc: Views.OperationDraftDocument
  draftResponse: Views.DraftEnvelopeResponse
  memberReviewResponse: Views.MemberReviewResponse
  revisionResponse: Views.RevisionDetailResponse
  taskPayloadResponse: Views.ConversionPlanPayload
  coverReportResponse: Views.CoverReport
  dirrename_Draft: Views.RenameDraftDocument
  draftMemberRequest: Views.DraftMember
  draftOverridesRequest: Views.OverrideSet
  editionSlotResponse: Views.WorkEditionSlot
  executionComponentResponse: Views.ExecutionComponent
  executionStartResponse: Views.StartExecutionResponse
  executionViewResponse: Views.ExecutionView
  generationReplayRevisionResponse: Views.StartGenerationResponse
  generationStartResponse: Views.StartGenerationResponse
  healthResponse: Views.HealthResponse
  memberConclusionResponse: Views.MemberConclusion
  memberTreeNodeResponse: Views.TreeNode
  recordCreateRequest: Views.CreateRecordInput
  refresh_Draft: Views.MetadataDraftDocument
  revisionMemberResponse: Views.RevisionMember
  workContentCopyResponse: Views.WorkContentCopy
  workContentDirNodeJSON: Views.WorkContentDir
  workContentResponse: Views.WorkContent
  workDetailResponse: Views.WorkDetail
  workListeningRequest: Views.PutWorkListeningParams
  workSummaryResponse: Views.WorkSummary
  cleanupRunEnvelopeResponse: WireSchemas['cleanupRunEnvelopeResponse'] & { run: Views.CleanupRun | null }
}

type BaseId<K> = K extends `${'conversion' | 'rename' | 'metadata_refresh'}_v1_${infer Base}` ? Base : K
type ViewId<K> = BaseId<K> extends 'executionFullViewResponse' | 'executionMembersViewResponse' | 'executionMemberViewResponse' ? 'executionViewResponse' : BaseId<K>
export type ViewOf<K extends SchemaId> = K extends unknown ? ViewId<K> extends keyof ViewBindings ? ViewBindings[ViewId<K>] : WireSchemas[K] : never
