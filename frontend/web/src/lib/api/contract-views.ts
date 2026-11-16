import type { SchemaId } from './contracts.gen'
import type * as Views from './views'
import type { WireSchemas } from './wire.gen'

// Wire bytes use generated types. These refinements describe the client's
// existing presentation vocabulary; field ownership stays with each wire root.
interface ViewBindings {
  updateCheckResponse: Views.UpdateCheck
  audioOutputSpecDTO: Views.AudioOutputSpec
  audioQualityDTO: Views.QualitySpec
  classifierCustomTagItem: Views.ClassifierCustomTag
  classifierTagLibraryResponse: Views.ClassifierTagLibraryResponse
  cleanupPreviewResponse: Views.CleanupPreview
  cleanupRunResponse: Views.CleanupRun
  collectionDetailResponse: Views.CollectionDetail
  collectionListResponse: Views.CollectionListResponse
  collectionResponse: Views.WorkCollection
  collectionWorksAppendResponse: Views.CollectionAppendResult
  conversion_DraftDoc: Views.OperationDraftDocument
  conversion_ExecutionOptions: Views.ExecutionOptions
  conversion_FrozenMemberConclusion: Views.ConversionMemberConclusion
  draftResponse: Views.DraftEnvelopeResponse
  memberReviewResponse: Views.MemberReviewResponse
  revisionResponse: Views.RevisionDetailResponse
  taskPayloadResponse: Views.ConversionPlanPayload
  copyResponse: Views.WorkCopy
  coverDestinationResponse: Views.CoverDelivery
  coverReportResponse: Views.CoverReport
  coverTargetResponse: Views.CoverTarget
  currentRecordResponse: Views.CurrentRecordResponse
  currentRevisionResponse: Views.CurrentRevisionSummary
  desiredProfileDTO: Views.DesiredProfile
  dirResponse: Views.LibraryDir
  dirrename_Draft: Views.RenameDraftDocument
  dirrename_Outcome: Views.RenameComponent
  dirrename_Result: Views.RenameTaskResult
  draftMemberRequest: Views.DraftMember
  draftOverridesRequest: Views.OverrideSet
  editionResponse: Views.WorkEdition
  editionSlotResponse: Views.WorkEditionSlot
  executionComponentResponse: Views.ExecutionComponent
  executionMemberSummaryResponse: Views.MemberSummary
  executionMembersResponse: Views.ExecutionMembers
  executionProgressEvent: Views.ExecutionProgressEvent
  executionProgressResponse: Views.ExecutionProgress
  executionRefResponse: Views.ExecutionRef
  executionStartResponse: Views.StartExecutionResponse
  executionViewResponse: Views.ExecutionView
  facetResponse: Views.WorkFacet
  fileOperationItem: Views.FileOperationItem
  fileOperationItemResponse: Views.FileOperationItemResult
  fileOperationRefreshResponse: Views.RefreshState
  fileOperationResponse: Views.FileOperationResult
  generationProgressResponse: Views.GenerationProgress
  generationReplayRevisionResponse: Views.StartGenerationResponse
  generationStartResponse: Views.StartGenerationResponse
  generationSummaryResponse: Views.GenerationSummary
  generationViewResponse: Views.GenerationView
  healthResponse: Views.HealthResponse
  libraryCreateRequest: Views.CreateLibraryInput
  libraryPatchRequest: Views.UpdateLibraryInput
  libraryRefResponse: Views.LibraryRef
  libraryResponse: Views.Library
  listeningResponse: Views.WorkListening
  listeningSummaryResponse: Views.WorkListeningSummary
  localCoverResponse: Views.LocalCover
  mediaFactsStatus: Views.MediaFactsStatus
  memberConclusionResponse: Views.MemberConclusion
  memberResponse: Views.WorksetMember
  memberTreeNodeResponse: Views.TreeNode
  operationResponse: Views.Operation
  outboundProxyResponse: Views.OutboundProxy
  preferencesResponse: Views.UserPreferences
  preferencesUpdateRequest: Views.UserPreferencesUpdate
  reconcile_ComponentOutcome: Views.ComponentOutcome
  reconcile_FileDecision: Views.FileDecision
  reconcile_Policy: Views.ResolvedPolicy
  reconcile_StepSummary: Views.StepSummary
  reconcile_VariantDecision: Views.VariantDecision
  recordCreateRequest: Views.CreateRecordInput
  recordCreateResponse: Views.CreateRecordResponse
  refresh_Draft: Views.MetadataDraftDocument
  refresh_Outcome: Views.MetadataComponent
  refresh_WorkResult: Views.MetadataTaskResult
  relationResponse: Views.WorkRelation
  relationWarningResponse: Views.WorkRelationWarning
  revisionCountsResponse: Views.RevisionCounts
  revisionMemberResponse: Views.RevisionMember
  rootValidationResponse: Views.RootValidation
  scanEventData: Views.ScanEventData
  skippedFolderResponse: Views.SkippedFolder
  sourceEntityResponse: Views.WorkSourceEntity
  tagFacetsResponse: Views.TagFacets
  taskClassifierSnapshot: Views.ClassifierSnapshot
  treeNodeResponse: Views.WorkRelationNode
  treeResponse: Views.MemberTreeResponse
  userDataResponse: Views.WorkUserData
  userTagResponse: Views.WorkUserTag
  workContentCopyResponse: Views.WorkContentCopy
  workContentDirNodeJSON: Views.WorkContentDir
  workContentFileNodeJSON: Views.WorkContentFile
  workContentResponse: Views.WorkContent
  workDetailResponse: Views.WorkDetail
  workListResponse: Views.WorkListResponse
  workListeningRequest: Views.PutWorkListeningParams
  workSummaryResponse: Views.WorkSummary
  workUserDataPatchRequest: Views.PatchWorkUserDataParams
  worksetListResponse: Views.WorksetListResponse
  worksetResponse: Views.Workset
  libraryListResponse: WireSchemas['libraryListResponse'] & { libraries: Views.Library[] | null }
  dirListResponse: WireSchemas['dirListResponse'] & { dirs: Views.LibraryDir[] | null }
  cleanupRunEnvelopeResponse: WireSchemas['cleanupRunEnvelopeResponse'] & { run: Views.CleanupRun | null }
  executionMemberEvent: WireSchemas['executionMemberEvent'] & { member: Views.MemberSummary }
  generationSnapshotEvent: Views.GenerationSnapshot
}

type BaseId<K> = K extends `${'conversion' | 'rename' | 'metadata_refresh'}_v1_${infer Base}` ? Base : K
type ViewId<K> = BaseId<K> extends 'executionFullViewResponse' | 'executionMembersViewResponse' | 'executionMemberViewResponse' ? 'executionViewResponse' : BaseId<K>
export type ViewOf<K extends SchemaId> = K extends unknown ? ViewId<K> extends keyof ViewBindings ? ViewBindings[ViewId<K>] : WireSchemas[K] : never
