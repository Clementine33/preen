import type * as Client from './types'
import type { WireExecutionEvent, WireGenerationEvent, WireMediaEvent, WireScanEvent, WireSchemas } from './wire.gen'

/** View refinements retain schema-owned fields; only presentation/domain literals are narrowed. */
type Keys<T> = T extends unknown ? keyof T : never
type Field<T, K> = T extends unknown ? K extends keyof T ? T[K] : never : never
type UnionView<T> = Pick<T, keyof T> & Partial<{ [K in Keys<T>]: Field<T, K> }>
type Refine<T, O extends Record<Exclude<keyof O, Keys<T>>, never>> = Omit<UnionView<T>, keyof O> & O

export type MediaFactsStatus = WireSchemas['mediaFactsStatus']

export type HealthResponse = Refine<WireSchemas['healthResponse'], { workset_max_members?: number }>

export type Library = WireSchemas['libraryResponse']

export type CreateLibraryInput = WireSchemas['libraryCreateRequest']

export type UpdateLibraryInput = WireSchemas['libraryPatchRequest']

export type LibraryDir = WireSchemas['dirResponse']

export type TreeNode = Refine<WireSchemas['memberTreeNodeResponse'], { children?: TreeNode[] }>

export type MemberTreeResponse = WireSchemas['treeResponse']

export type FileOperationItem = WireSchemas['fileOperationItem']

export type FileOperationItemResult = WireSchemas['fileOperationItemResponse']

export type RefreshState = WireSchemas['fileOperationRefreshResponse']

export type FileOperationResult = WireSchemas['fileOperationResponse']

export type ScanEventData = WireSchemas['scanEventData']

export type QualitySpec = WireSchemas['audioQualityDTO']

export type AudioOutputSpec = WireSchemas['audioOutputSpecDTO']

export type PolicyProfile = WireSchemas['desiredProfileDTO']

export type ResolvedPolicy = WireSchemas['reconcile_Policy']

export type ClassifierCustomTag = WireSchemas['classifierCustomTagItem']

export type ClassifierTagLibraryResponse = WireSchemas['classifierTagLibraryResponse']

export type UserPreferences = WireSchemas['preferencesResponse']

export type UserPreferencesUpdate = WireSchemas['preferencesUpdateRequest']

export type CleanupPreview = Refine<WireSchemas['cleanupPreviewResponse'], { candidates: Partial<Record<Client.CleanupCategory, number>>, rules: Partial<Record<Client.CleanupCategory, { protection: string }>> }>

export type CleanupRun = Refine<WireSchemas['cleanupRunResponse'], { counts: Partial<Record<Client.CleanupCategory, { checked: number, deleted: number, skipped: number }>> }>

export type OutboundProxy = WireSchemas['outboundProxyResponse']

export type WorkUserTag = WireSchemas['userTagResponse']

export type LocalCover = WireSchemas['localCoverResponse']

export type WorkSummary = Refine<WireSchemas['workSummaryResponse'], { user_tags: WorkUserTag[], listening: WorkListeningSummary, local_cover?: LocalCover | null }>

export type WorkListeningSummary = WireSchemas['listeningSummaryResponse']

export type WorkListening = WireSchemas['listeningResponse']

export type PutWorkListeningParams = Required<Omit<WireSchemas['workListeningRequest'], 'finished'>> & Pick<WireSchemas['workListeningRequest'], 'finished'>

export type WorkSourceEntity = WireSchemas['sourceEntityResponse']

export type WorkEdition = WireSchemas['editionResponse']

export type WorkRelation = WireSchemas['relationResponse']

export type WorkCopy = WireSchemas['copyResponse']

export type WorkEditionNode = Refine<WireSchemas['treeNodeResponse'], { role: 'original' | 'edition_page' | 'translation', relation: WorkRelation }>

export type WorkRelationNode = WireSchemas['treeNodeResponse']

export type WorkRelationWarning = WireSchemas['relationWarningResponse']

export type WorkEditionSlot = Refine<WireSchemas['editionSlotResponse'], { root: WorkEditionNode, translations: WorkEditionNode[] }>

export type WorkDetail = Refine<WireSchemas['workDetailResponse'], { voice_actors: WorkSourceEntity[], source_tags: WorkSourceEntity[], language_editions: WorkEdition[], edition_tree: WorkEditionSlot[], relation: WorkRelation, relation_warnings: WorkRelationWarning[], relation_nodes: WorkRelationNode[], copies: WorkCopy[], listening: WorkListening[], local_cover?: LocalCover | null, age_category: WorkSummary['age_category'], user_tags: WorkUserTag[] }>

export type WorkListResponse = WireSchemas['workListResponse']

export type WorkCollection = WireSchemas['collectionResponse']

export type CollectionListResponse = WireSchemas['collectionListResponse']

export type CollectionDetail = Refine<WireSchemas['collectionDetailResponse'], { works: WorkSummary[] }>

export type CollectionAppendResult = WireSchemas['collectionWorksAppendResponse']

export type WorkFacet = WireSchemas['facetResponse']

export type TagFacets = WireSchemas['tagFacetsResponse']

export type WorkUserData = WireSchemas['userDataResponse']

export type PatchWorkUserDataParams = WireSchemas['workUserDataPatchRequest']

export type WorkContentFile = WireSchemas['workContentFileNodeJSON']

export type WorkContentDir = Refine<WireSchemas['workContentDirNodeJSON'], { children: Array<WorkContentDir | WorkContentFile> }>

export type WorkContentCopy = Refine<WireSchemas['workContentCopyResponse'], { tree?: WorkContentDir }>

export type WorkContent = Refine<WireSchemas['workContentResponse'], { copies: WorkContentCopy[] }>

export type LibraryRef = WireSchemas['libraryRefResponse']

export type WorksetMember = WireSchemas['memberResponse']

export type RevisionCounts = WireSchemas['revisionCountsResponse']

export type CurrentRevisionSummary = WireSchemas['currentRevisionResponse']

export type GenerationProgress = WireSchemas['generationProgressResponse']

export type GenerationSummary = WireSchemas['generationSummaryResponse']

export type Operation = WireSchemas['operationResponse']

export type Workset = WireSchemas['worksetResponse']

export type WorksetListResponse = WireSchemas['worksetListResponse']

export type SkippedFolder = WireSchemas['skippedFolderResponse']

export type CreateRecordInput = Refine<WireSchemas['recordCreateRequest'], { selection: Client.RecordSelection }>

export type CreateRecordResponse = WireSchemas['recordCreateResponse']

export type CurrentRecordResponse = WireSchemas['currentRecordResponse']

export type DesiredProfile = WireSchemas['desiredProfileDTO']

export type OverrideSet = Refine<NonNullable<NonNullable<WireSchemas['conversion_DraftDoc']['members']>[number]['overrides']>, { matched?: DesiredProfile, unmatched?: DesiredProfile, other?: DesiredProfile }>

export type DraftMember = Refine<NonNullable<WireSchemas['conversion_DraftDoc']['members']>[number], { overrides?: OverrideSet }>

export type OperationDraftDocument = Refine<WireSchemas['conversion_DraftDoc'], { matched: DesiredProfile, unmatched: DesiredProfile, other?: DesiredProfile, members: DraftMember[], no_effect_tags: string[], other_tags?: string[] }>

export type TaskDraftMember = NonNullable<WireSchemas['refresh_Draft']['members']>[number]

export type RenameDraftMember = NonNullable<WireSchemas['dirrename_Draft']['members']>[number]

export type RenameDraftDocument = Refine<WireSchemas['dirrename_Draft'], { members: RenameDraftMember[] }>

export type MetadataDraftDocument = Refine<WireSchemas['refresh_Draft'], { members: TaskDraftMember[] }>

export type DraftEnvelopeResponse = Refine<WireSchemas['conversion_v1_draftResponse'], { task: Client.TaskEnvelope<OperationDraftDocument> }>

export type GenerationView = WireSchemas['generationViewResponse']

export type ExecutionOptions = WireSchemas['conversion_ExecutionOptions']

export type ExecutionRef = WireSchemas['executionRefResponse']

export type ExecutionProgress = WireSchemas['executionProgressResponse']

export type ExecutionComponent = Refine<WireSchemas['executionComponentResponse'], { task_result?: Field<WireSchemas['executionComponentResponse'], 'task_result'>, kept_decisions: FileDecision[] }>

export type MemberSummary = WireSchemas['executionMemberSummaryResponse']

export type ExecutionMembers = WireSchemas['executionMembersResponse']

export type ExecutionView = Refine<WireSchemas['executionViewResponse'], { options?: ExecutionOptions, components?: ExecutionComponent[], members?: ExecutionMembers, member?: MemberSummary }>

export type ExecutionProgressEvent = WireSchemas['executionProgressEvent']

export type StartExecutionResponse = Refine<WireSchemas['executionStartResponse'], { execution: ExecutionView }>

export type RootValidation = WireSchemas['rootValidationResponse']

export type FileDecision = WireSchemas['reconcile_FileDecision']

export type VariantDecision = WireSchemas['reconcile_VariantDecision']

export type LaneDecision = WireSchemas['reconcile_ComponentOutcome']['lanes'][number]

export type PlanOperation = WireSchemas['reconcile_ComponentOutcome']['operations'][number]

export type ComponentOutcome = WireSchemas['reconcile_ComponentOutcome']

export type StepSummary = WireSchemas['reconcile_StepSummary']

export type ClassifierSnapshot = WireSchemas['taskClassifierSnapshot']

export type ConversionPlanPayload = Refine<WireSchemas['conversion_v1_taskPayloadResponse'], { classifier: ClassifierSnapshot, summary: StepSummary, components: ComponentOutcome[] }>

export type RenameComponent = WireSchemas['dirrename_Outcome']

export type CoverTarget = WireSchemas['coverTargetResponse']

export type MetadataComponent = WireSchemas['refresh_Outcome']

export type MetadataIssue = NonNullable<WireSchemas['refresh_WorkResult']['issues']>[number]

export type MetadataTaskResult = WireSchemas['refresh_WorkResult']

export type CoverDelivery = WireSchemas['coverDestinationResponse']

export type CoverReport = Refine<WireSchemas['coverReportResponse'], { items: CoverDelivery[], counts: Record<Client.CoverStatus, number> }>

export type ConversionPartitionConclusion = WireSchemas['conversion_FrozenMemberConclusion']['matched']

export type ConversionMemberConclusion = WireSchemas['conversion_FrozenMemberConclusion']

export type MemberConclusion<T = ConversionMemberConclusion> = Refine<WireSchemas['memberConclusionResponse'], { task?: T }>

export type RevisionMember<T = ConversionMemberConclusion> = Refine<WireSchemas['revisionMemberResponse'], { conclusion: MemberConclusion<T>, sources: Record<string, string> }>

export type MemberReviewResponse<P = ConversionPlanPayload, T = ConversionMemberConclusion> = Refine<WireSchemas['conversion_v1_memberReviewResponse'], { effective: ResolvedPolicy | null, roots: RootValidation[], task: Client.TaskEnvelope<P>, conclusion: MemberConclusion<T> }>

export type RevisionDetailResponse<P = Omit<ConversionPlanPayload, 'components'>, T = ConversionMemberConclusion> = Refine<WireSchemas['conversion_v1_revisionResponse'], { task: Client.TaskEnvelope<P>, counts: RevisionCounts, members: MemberConclusion<T>[], category_counts: Record<string, number>, execution: ExecutionRef | null }>

export type RenameTaskResult = WireSchemas['dirrename_Result']

export type MediaFactsEvent = WireMediaEvent extends infer E ? E extends { type: 'status' } ? Omit<E, 'data'> & { data: MediaFactsStatus } : E : never

export type ScanEvent = WireScanEvent

export type UpdateCheck = Refine<WireSchemas['updateCheckResponse'], { state: 'checking' | 'none' }> | Refine<WireSchemas['updateCheckResponse'], { state: 'available', latest_version: string, release_url: string }>

export type DraftResponse<D = OperationDraftDocument> = Omit<DraftEnvelopeResponse, 'task'> & Pick<WireSchemas['taskDraftEnvelope'], 'schema_version'> & { document: D }

export type StartGenerationResponse = Refine<WireSchemas['generationStartResponse'], { generation: GenerationView }> | Refine<WireSchemas['generationReplayRevisionResponse'], { created: false, revision: CurrentRevisionSummary }>

export type GenerationEvent = WireGenerationEvent extends infer E ? E extends { type: 'session_snapshot' } ? Omit<E, 'data'> & { data: GenerationView } : E : never

export type ExecutionEvent = WireExecutionEvent extends infer E ? E extends { type: 'execution_snapshot' } ? Omit<E, 'data'> & { data: ExecutionView } : E extends { type: 'component' } ? Omit<E, 'data'> & { data: ExecutionComponent } : E extends { type: 'progress' } ? Omit<E, 'data'> & { data: ExecutionProgressEvent } : E extends { type: 'member' } ? Omit<E, 'data'> & { data: Refine<WireSchemas['executionMemberEvent'], { member: MemberSummary, counts: Client.MemberCounts }> } : E : never

export type TaskEnvelope<P> = Refine<WireSchemas['taskDraftEnvelope'], { payload: P }>

export type FileOperationInput = WireSchemas['fileOperationRequest']

export type MemberCounts = WireSchemas['executionMemberCountsResponse']
export type RecordSelection = Refine<WireSchemas['selectionRequest'], { all: true, except_dir_ids: string[] }> | Refine<WireSchemas['selectionRequest'], { dir_ids: string[] }>
export type ExecutionScope = Pick<Required<WireSchemas['executionStartRequest']>, 'member_ids'> | Pick<Required<WireSchemas['executionStartRequest']>, 'all_except_member_ids'>
