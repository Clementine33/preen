// Generated HTTP status and SSE event bindings. Do not edit.
import type { WireSchemas } from './wire.gen'
export type SchemaId = keyof WireSchemas
export const schemaIds = ["appliedGroupResponse","audioOutputSpecDTO","audioQualityDTO","circleFacetsResponse","classifierCustomTagItem","classifierTagCreateRequest","classifierTagLibraryResponse","cleanupCategoriesRequest","cleanupCountResponse","cleanupPreviewResponse","cleanupRuleResponse","cleanupRunEnvelopeResponse","cleanupRunResponse","collectionDetailResponse","collectionListResponse","collectionNameRequest","collectionResponse","collectionWorkMoveRequest","collectionWorksAppendRequest","collectionWorksAppendResponse","conversion_CommonSettings","conversion_CommonSettings_request","conversion_DraftDoc","conversion_DraftDoc_request","conversion_ExecutionOptions","conversion_FrozenMemberConclusion","conversion_v1_draftResponse","conversion_v1_executionComponentResponse","conversion_v1_executionFullViewResponse","conversion_v1_executionMemberViewResponse","conversion_v1_executionMembersViewResponse","conversion_v1_executionProgressResponse","conversion_v1_executionStartResponse","conversion_v1_executionViewResponse","conversion_v1_memberConclusionResponse","conversion_v1_memberReviewResponse","conversion_v1_operationResponse","conversion_v1_revisionMemberResponse","conversion_v1_revisionResponse","conversion_v1_taskDraftEnvelope","conversion_v1_taskEnvelopeResponse","conversion_v1_taskPayloadResponse","conversion_v1_taskSettingsRequest","conversion_v1_taskSettingsResponse","copyResponse","coverDestinationResponse","coverReportResponse","coverTargetResponse","currentRecordResponse","currentRevisionResponse","desiredProfileDTO","dirListResponse","dirResponse","dirrename_Common","dirrename_Common_request","dirrename_Draft","dirrename_Draft_request","dirrename_Outcome","dirrename_Result","draftDocumentRequest","draftMemberRequest","draftOverridesRequest","draftResponse","editionResponse","editionSlotResponse","emptyEvent","emptyRequest","errorResponse","errorWithDetails","executionComponentResponse","executionFailedEvent","executionFullViewResponse","executionMemberCountsResponse","executionMemberEvent","executionMemberSummaryResponse","executionMemberViewResponse","executionMembersResponse","executionMembersViewResponse","executionProgressEvent","executionProgressResponse","executionRefResponse","executionStartRequest","executionStartResponse","executionStoppedEvent","executionSucceededEvent","executionViewResponse","facetResponse","fileOperationItem","fileOperationItemResponse","fileOperationRefreshResponse","fileOperationRequest","fileOperationResponse","generationCompletedEvent","generationFailedEvent","generationProgressEvent","generationProgressResponse","generationReplayRevisionResponse","generationStartResponse","generationStoppedEvent","generationSummaryResponse","generationViewResponse","healthResponse","ignoredGroupResponse","importResponse","libraryCreateRequest","libraryListResponse","libraryPatchRequest","libraryRefResponse","libraryResponse","listeningResponse","listeningSummaryResponse","localCoverResponse","mediaFactsLibraryStat","mediaFactsStatus","memberConclusionResponse","memberResponse","memberReviewResponse","memberTreeNodeResponse","metadata_refresh_v1_draftResponse","metadata_refresh_v1_executionComponentResponse","metadata_refresh_v1_executionFullViewResponse","metadata_refresh_v1_executionMemberViewResponse","metadata_refresh_v1_executionMembersViewResponse","metadata_refresh_v1_executionProgressResponse","metadata_refresh_v1_executionStartResponse","metadata_refresh_v1_executionViewResponse","metadata_refresh_v1_memberConclusionResponse","metadata_refresh_v1_memberReviewResponse","metadata_refresh_v1_operationResponse","metadata_refresh_v1_revisionMemberResponse","metadata_refresh_v1_revisionResponse","metadata_refresh_v1_taskDraftEnvelope","metadata_refresh_v1_taskEnvelopeResponse","metadata_refresh_v1_taskPayloadResponse","metadata_refresh_v1_taskSettingsRequest","metadata_refresh_v1_taskSettingsResponse","operationResponse","outboundProxyPutRequest","outboundProxyResponse","planSummaryResponse","preferencesResponse","preferencesUpdateRequest","randomWorkResponse","reconcile_ComponentOutcome","reconcile_FileDecision","reconcile_Policy","reconcile_StepSummary","reconcile_VariantDecision","recordCreateRequest","recordCreateResponse","refresh_Common","refresh_Common_request","refresh_Draft","refresh_Draft_request","refresh_Outcome","refresh_PlanSummary","refresh_WorkResult","relationResponse","relationWarningResponse","rename_v1_draftResponse","rename_v1_executionComponentResponse","rename_v1_executionFullViewResponse","rename_v1_executionMemberViewResponse","rename_v1_executionMembersViewResponse","rename_v1_executionProgressResponse","rename_v1_executionStartResponse","rename_v1_executionViewResponse","rename_v1_memberConclusionResponse","rename_v1_memberReviewResponse","rename_v1_operationResponse","rename_v1_revisionMemberResponse","rename_v1_revisionResponse","rename_v1_taskDraftEnvelope","rename_v1_taskEnvelopeResponse","rename_v1_taskPayloadResponse","rename_v1_taskSettingsRequest","rename_v1_taskSettingsResponse","revisionCountsResponse","revisionMemberResponse","revisionResponse","rootValidationResponse","scanEventData","scanRequest","selectionRequest","sessionCreateRequest","sessionStatusEvent","skippedFolderResponse","sourceEntityResponse","sseError","tagFacetsResponse","taskClassifierSnapshot","taskDraftEnvelope","taskEnvelopeResponse","taskPayloadResponse","taskSettingsRequest","taskSettingsResponse","treeNodeResponse","treeResponse","updateCheckResponse","userDataResponse","userTagListResponse","userTagRequest","userTagResponse","voiceActorFacetsResponse","warningResponse","workContentCopyResponse","workContentDirNodeJSON","workContentFileNodeJSON","workContentNodeResponse","workContentResponse","workDetailResponse","workImportCircleRequest","workImportCoreRequest","workImportEditionItemRequest","workImportEditionsRequest","workImportRelationsRequest","workImportStatisticsRequest","workImportTagRequest","workImportVoiceActorRequest","workListResponse","workListeningRequest","workMetadataImportRequest","workSummaryResponse","workUserDataPatchRequest","workUserTagsRequest","worksetListResponse","worksetPatchRequest","worksetResponse","workset_PlanSummary"] as const satisfies readonly SchemaId[]
export const httpContracts = {
  "DELETE /api/v1/classifier-tags/{id}": {
    "request": "",
    "responses": {
      "204": "@empty"
    }
  },
  "DELETE /api/v1/collections/{id}": {
    "request": "",
    "responses": {
      "204": "@empty"
    }
  },
  "DELETE /api/v1/collections/{id}/works/{rj}": {
    "request": "",
    "responses": {
      "204": "@empty"
    }
  },
  "DELETE /api/v1/libraries/{id}": {
    "request": "",
    "responses": {
      "204": "@empty"
    }
  },
  "DELETE /api/v1/session": {
    "request": "",
    "responses": {
      "204": "@empty"
    }
  },
  "DELETE /api/v1/user-tags/{id}": {
    "request": "",
    "responses": {
      "204": "@empty"
    }
  },
  "DELETE /api/v1/works/{rj}": {
    "request": "",
    "responses": {
      "204": "@empty"
    }
  },
  "DELETE /api/v1/works/{rj}/listening": {
    "request": "",
    "responses": {
      "204": "@empty"
    }
  },
  "GET /api/v1/circles": {
    "request": "",
    "responses": {
      "200": "circleFacetsResponse"
    }
  },
  "GET /api/v1/classifier-tags": {
    "request": "",
    "responses": {
      "200": "classifierTagLibraryResponse"
    }
  },
  "GET /api/v1/collections": {
    "request": "",
    "responses": {
      "200": "collectionListResponse"
    }
  },
  "GET /api/v1/collections/{id}": {
    "request": "",
    "responses": {
      "200": "collectionDetailResponse"
    }
  },
  "GET /api/v1/health": {
    "request": "",
    "responses": {
      "200": "healthResponse"
    }
  },
  "GET /api/v1/inventory/media-facts/events": {
    "request": "",
    "responses": {
      "200": "@sse"
    }
  },
  "GET /api/v1/libraries": {
    "request": "",
    "responses": {
      "200": "libraryListResponse"
    }
  },
  "GET /api/v1/libraries/{id}": {
    "request": "",
    "responses": {
      "200": "libraryResponse"
    }
  },
  "GET /api/v1/libraries/{id}/dirs": {
    "request": "",
    "responses": {
      "200": "dirListResponse"
    }
  },
  "GET /api/v1/libraries/{id}/operations/{type}/current": {
    "request": "",
    "responses": {
      "200": "currentRecordResponse"
    }
  },
  "GET /api/v1/libraries/{id}/tree": {
    "request": "",
    "responses": {
      "200": "treeResponse"
    }
  },
  "GET /api/v1/maintenance/cleanup": {
    "request": "",
    "responses": {
      "200": "cleanupRunEnvelopeResponse"
    }
  },
  "GET /api/v1/outbound-proxy": {
    "request": "",
    "responses": {
      "200": "outboundProxyResponse"
    }
  },
  "GET /api/v1/preferences": {
    "request": "",
    "responses": {
      "200": "preferencesResponse"
    }
  },
  "GET /api/v1/tags": {
    "request": "",
    "responses": {
      "200": "tagFacetsResponse"
    }
  },
  "GET /api/v1/tasks/{type}/settings": {
    "request": "",
    "responses": {
      "200": "taskSettingsResponse"
    }
  },
  "GET /api/v1/update-check": {
    "request": "",
    "responses": {
      "200": "updateCheckResponse"
    }
  },
  "GET /api/v1/user-tags": {
    "request": "",
    "responses": {
      "200": "userTagListResponse"
    }
  },
  "GET /api/v1/voice-actors": {
    "request": "",
    "responses": {
      "200": "voiceActorFacetsResponse"
    }
  },
  "GET /api/v1/works": {
    "request": "",
    "responses": {
      "200": "workListResponse"
    }
  },
  "GET /api/v1/works/random": {
    "request": "",
    "responses": {
      "200": "randomWorkResponse"
    }
  },
  "GET /api/v1/works/{rj}": {
    "request": "",
    "responses": {
      "200": "workDetailResponse"
    }
  },
  "GET /api/v1/works/{rj}/content": {
    "request": "",
    "responses": {
      "200": "workContentResponse"
    }
  },
  "GET /api/v1/worksets": {
    "request": "",
    "responses": {
      "200": "worksetListResponse"
    }
  },
  "GET /api/v1/worksets/{id}": {
    "request": "",
    "responses": {
      "200": "worksetResponse"
    }
  },
  "GET /api/v1/worksets/{id}/operations/{type}": {
    "request": "",
    "responses": {
      "200": "operationResponse"
    }
  },
  "GET /api/v1/worksets/{id}/operations/{type}/draft": {
    "request": "",
    "responses": {
      "200": "draftResponse"
    }
  },
  "GET /api/v1/worksets/{id}/operations/{type}/executions/{executionId}": {
    "request": "",
    "responses": {
      "200": "executionViewResponse"
    }
  },
  "GET /api/v1/worksets/{id}/operations/{type}/executions/{executionId}/covers": {
    "request": "",
    "responses": {
      "200": "coverReportResponse"
    }
  },
  "GET /api/v1/worksets/{id}/operations/{type}/executions/{executionId}/events": {
    "request": "",
    "responses": {
      "200": "@sse"
    }
  },
  "GET /api/v1/worksets/{id}/operations/{type}/planning-sessions/{genId}": {
    "request": "",
    "responses": {
      "200": "generationViewResponse"
    }
  },
  "GET /api/v1/worksets/{id}/operations/{type}/planning-sessions/{genId}/events": {
    "request": "",
    "responses": {
      "200": "@sse"
    }
  },
  "GET /api/v1/worksets/{id}/operations/{type}/revisions/{planId}": {
    "request": "",
    "responses": {
      "200": "revisionResponse"
    }
  },
  "GET /api/v1/worksets/{id}/operations/{type}/revisions/{planId}/members/{memberId}": {
    "request": "",
    "responses": {
      "200": "memberReviewResponse"
    }
  },
  "PATCH /api/v1/collections/{id}": {
    "request": "collectionNameRequest",
    "responses": {
      "200": "collectionResponse"
    }
  },
  "PATCH /api/v1/collections/{id}/works/{rj}": {
    "request": "collectionWorkMoveRequest",
    "responses": {
      "204": "@empty"
    }
  },
  "PATCH /api/v1/libraries/{id}": {
    "request": "libraryPatchRequest",
    "responses": {
      "200": "libraryResponse"
    }
  },
  "PATCH /api/v1/user-tags/{id}": {
    "request": "userTagRequest",
    "responses": {
      "200": "userTagResponse"
    }
  },
  "PATCH /api/v1/works/{rj}/user-data": {
    "request": "workUserDataPatchRequest",
    "responses": {
      "200": "userDataResponse"
    }
  },
  "PATCH /api/v1/worksets/{id}": {
    "request": "worksetPatchRequest",
    "responses": {
      "200": "worksetResponse"
    }
  },
  "POST /api/v1/classifier-tags": {
    "request": "classifierTagCreateRequest",
    "responses": {
      "201": "classifierCustomTagItem"
    }
  },
  "POST /api/v1/collections": {
    "request": "collectionNameRequest",
    "responses": {
      "201": "collectionResponse"
    }
  },
  "POST /api/v1/collections/{id}/works": {
    "request": "collectionWorksAppendRequest",
    "responses": {
      "200": "collectionWorksAppendResponse"
    }
  },
  "POST /api/v1/libraries": {
    "request": "libraryCreateRequest",
    "responses": {
      "201": "libraryResponse"
    }
  },
  "POST /api/v1/libraries/{id}/file-operations": {
    "request": "fileOperationRequest",
    "responses": {
      "200": "fileOperationResponse"
    }
  },
  "POST /api/v1/libraries/{id}/scans": {
    "request": "scanRequest",
    "responses": {
      "200": "@sse"
    }
  },
  "POST /api/v1/libraries/{id}/tree/refresh": {
    "request": "",
    "responses": {
      "200": "treeResponse"
    }
  },
  "POST /api/v1/maintenance/cleanup": {
    "request": "cleanupCategoriesRequest",
    "responses": {
      "202": "cleanupRunEnvelopeResponse"
    }
  },
  "POST /api/v1/maintenance/cleanup/preview": {
    "request": "cleanupCategoriesRequest",
    "responses": {
      "200": "cleanupPreviewResponse"
    }
  },
  "POST /api/v1/maintenance/cleanup/{runId}/cancel": {
    "request": "emptyRequest",
    "responses": {
      "200": "cleanupRunEnvelopeResponse"
    }
  },
  "POST /api/v1/session": {
    "request": "sessionCreateRequest",
    "responses": {
      "204": "@empty"
    }
  },
  "POST /api/v1/user-tags": {
    "request": "userTagRequest",
    "responses": {
      "200": "userTagResponse"
    }
  },
  "POST /api/v1/worksets/{id}/operations/{type}/executions/{executionId}/cancel": {
    "request": "",
    "responses": {
      "200": "executionViewResponse"
    }
  },
  "POST /api/v1/worksets/{id}/operations/{type}/executions/{executionId}/covers/cancel": {
    "request": "",
    "responses": {
      "200": "coverReportResponse"
    }
  },
  "POST /api/v1/worksets/{id}/operations/{type}/executions/{executionId}/covers/retry": {
    "request": "",
    "responses": {
      "200": "coverReportResponse"
    }
  },
  "POST /api/v1/worksets/{id}/operations/{type}/planning-sessions/{genId}/cancel": {
    "request": "",
    "responses": {
      "200": "generationViewResponse"
    }
  },
  "POST /api/v1/worksets/{id}/operations/{type}/revisions": {
    "request": "emptyRequest",
    "responses": {
      "200": "generationReplayRevisionResponse",
      "202": "generationStartResponse"
    }
  },
  "POST /api/v1/worksets/{id}/operations/{type}/revisions/{planId}/executions": {
    "request": "executionStartRequest",
    "responses": {
      "200": "executionStartResponse",
      "202": "executionStartResponse"
    }
  },
  "PUT /api/v1/libraries/{id}/operations/{type}/current": {
    "request": "recordCreateRequest",
    "responses": {
      "200": "recordCreateResponse",
      "201": "recordCreateResponse"
    }
  },
  "PUT /api/v1/outbound-proxy": {
    "request": "outboundProxyPutRequest",
    "responses": {
      "200": "outboundProxyResponse"
    }
  },
  "PUT /api/v1/preferences": {
    "request": "preferencesUpdateRequest",
    "responses": {
      "200": "preferencesResponse"
    }
  },
  "PUT /api/v1/tasks/{type}/settings": {
    "request": "taskSettingsRequest",
    "responses": {
      "200": "taskSettingsResponse"
    }
  },
  "PUT /api/v1/works/{rj}/listening": {
    "request": "workListeningRequest",
    "responses": {
      "200": "listeningResponse"
    }
  },
  "PUT /api/v1/works/{rj}/metadata": {
    "request": "workMetadataImportRequest",
    "responses": {
      "200": "importResponse"
    }
  },
  "PUT /api/v1/works/{rj}/user-tags": {
    "request": "workUserTagsRequest",
    "responses": {
      "200": "userTagListResponse"
    }
  },
  "PUT /api/v1/worksets/{id}/operations/{type}/draft": {
    "request": "draftDocumentRequest",
    "responses": {
      "200": "operationResponse"
    }
  }
} as const satisfies Record<string, { request: SchemaId | ''; responses: Record<string, SchemaId | '@empty' | '@sse'> }>
export const sseContracts = {
  "execution": {
    "canceled": "executionStoppedEvent",
    "component": "executionComponentResponse",
    "error": "sseError",
    "execution_snapshot": "executionViewResponse",
    "failed": "executionFailedEvent",
    "interrupted": "executionStoppedEvent",
    "member": "executionMemberEvent",
    "message": "sessionStatusEvent",
    "progress": "executionProgressEvent",
    "succeeded": "executionSucceededEvent"
  },
  "generation": {
    "canceled": "generationStoppedEvent",
    "completed": "generationCompletedEvent",
    "error": "sseError",
    "failed": "generationFailedEvent",
    "interrupted": "generationStoppedEvent",
    "message": "sessionStatusEvent",
    "progress": "generationProgressEvent",
    "session_snapshot": "generationViewResponse"
  },
  "media": {
    "changed": "emptyEvent",
    "status": "mediaFactsStatus",
    "works_changed": "emptyEvent"
  },
  "scan": {
    "cancelled": "scanEventData",
    "completed": "scanEventData",
    "error": "scanEventData",
    "progress": "scanEventData",
    "started": "scanEventData"
  }
} as const satisfies Record<string, Record<string, SchemaId>>
export const taskSchemas = {
  "conversion": {
    "draftResponse": "conversion_v1_draftResponse",
    "executionComponentResponse": "conversion_v1_executionComponentResponse",
    "executionFullViewResponse": "conversion_v1_executionFullViewResponse",
    "executionMemberViewResponse": "conversion_v1_executionMemberViewResponse",
    "executionMembersViewResponse": "conversion_v1_executionMembersViewResponse",
    "executionProgressResponse": "conversion_v1_executionProgressResponse",
    "executionStartResponse": "conversion_v1_executionStartResponse",
    "executionViewResponse": "conversion_v1_executionViewResponse",
    "memberConclusionResponse": "conversion_v1_memberConclusionResponse",
    "memberReviewResponse": "conversion_v1_memberReviewResponse",
    "operationResponse": "conversion_v1_operationResponse",
    "revisionMemberResponse": "conversion_v1_revisionMemberResponse",
    "revisionResponse": "conversion_v1_revisionResponse",
    "taskDraftEnvelope": "conversion_v1_taskDraftEnvelope",
    "taskEnvelopeResponse": "conversion_v1_taskEnvelopeResponse",
    "taskPayloadResponse": "conversion_v1_taskPayloadResponse",
    "taskSettingsRequest": "conversion_v1_taskSettingsRequest",
    "taskSettingsResponse": "conversion_v1_taskSettingsResponse",
    "draftDocumentRequest": "draftDocumentRequest"
  },
  "metadata_refresh": {
    "draftResponse": "metadata_refresh_v1_draftResponse",
    "executionComponentResponse": "metadata_refresh_v1_executionComponentResponse",
    "executionFullViewResponse": "metadata_refresh_v1_executionFullViewResponse",
    "executionMemberViewResponse": "metadata_refresh_v1_executionMemberViewResponse",
    "executionMembersViewResponse": "metadata_refresh_v1_executionMembersViewResponse",
    "executionProgressResponse": "metadata_refresh_v1_executionProgressResponse",
    "executionStartResponse": "metadata_refresh_v1_executionStartResponse",
    "executionViewResponse": "metadata_refresh_v1_executionViewResponse",
    "memberConclusionResponse": "metadata_refresh_v1_memberConclusionResponse",
    "memberReviewResponse": "metadata_refresh_v1_memberReviewResponse",
    "operationResponse": "metadata_refresh_v1_operationResponse",
    "revisionMemberResponse": "metadata_refresh_v1_revisionMemberResponse",
    "revisionResponse": "metadata_refresh_v1_revisionResponse",
    "taskDraftEnvelope": "metadata_refresh_v1_taskDraftEnvelope",
    "taskEnvelopeResponse": "metadata_refresh_v1_taskEnvelopeResponse",
    "taskPayloadResponse": "metadata_refresh_v1_taskPayloadResponse",
    "taskSettingsRequest": "metadata_refresh_v1_taskSettingsRequest",
    "taskSettingsResponse": "metadata_refresh_v1_taskSettingsResponse",
    "draftDocumentRequest": "refresh_Draft_request"
  },
  "rename": {
    "draftResponse": "rename_v1_draftResponse",
    "executionComponentResponse": "rename_v1_executionComponentResponse",
    "executionFullViewResponse": "rename_v1_executionFullViewResponse",
    "executionMemberViewResponse": "rename_v1_executionMemberViewResponse",
    "executionMembersViewResponse": "rename_v1_executionMembersViewResponse",
    "executionProgressResponse": "rename_v1_executionProgressResponse",
    "executionStartResponse": "rename_v1_executionStartResponse",
    "executionViewResponse": "rename_v1_executionViewResponse",
    "memberConclusionResponse": "rename_v1_memberConclusionResponse",
    "memberReviewResponse": "rename_v1_memberReviewResponse",
    "operationResponse": "rename_v1_operationResponse",
    "revisionMemberResponse": "rename_v1_revisionMemberResponse",
    "revisionResponse": "rename_v1_revisionResponse",
    "taskDraftEnvelope": "rename_v1_taskDraftEnvelope",
    "taskEnvelopeResponse": "rename_v1_taskEnvelopeResponse",
    "taskPayloadResponse": "rename_v1_taskPayloadResponse",
    "taskSettingsRequest": "rename_v1_taskSettingsRequest",
    "taskSettingsResponse": "rename_v1_taskSettingsResponse",
    "draftDocumentRequest": "dirrename_Draft_request"
  }
} as const satisfies Record<string, Record<string, SchemaId>>
