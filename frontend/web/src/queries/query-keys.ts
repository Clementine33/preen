// Central query-key factories. Pages and composables must never construct
// keys ad hoc: cache isolation, prefix invalidation, and stale-response
// protection all depend on these exact tuple shapes.
import type { CollectionWindow, ListWorksParams, WorkFacetKind, WorksScope } from '@/lib/api/types'

export const queryKeys = {
  libraries: {
    all: () => ['libraries'] as const,
    list: () => ['libraries', 'list'] as const,
    // The overview listing: every direct child directory of the root. Its
    // identity is the library-relative path, and the key carries the root
    // identity so a genuine root change invalidates it.
    dirsPrefix: (libraryId: string) => ['libraries', 'dirs', libraryId] as const,
    dirs: (libraryId: string, rootIdentity: string) =>
      ['libraries', 'dirs', libraryId, rootIdentity] as const,
    // One member tree, addressed by the member's directory identity: the
    // overview and the conversion entry derive the same identity for the same
    // directory, so they share this entry instead of reading the tree twice.
    memberTreesPrefix: (libraryId: string) => ['libraries', 'member-trees', libraryId] as const,
    memberTreesAll: () => ['libraries', 'member-trees'] as const,
    memberTree: (libraryId: string, rootIdentity: string, dirId: string) =>
      ['libraries', 'member-trees', libraryId, rootIdentity, dirId] as const,
  },
  plans: {
    lists: () => ['plans', 'list'] as const,
    libraryPrefix: (libraryId: string) => ['plans', 'list', libraryId] as const,
    list: (libraryId: string, limit = 100) => ['plans', 'list', libraryId, limit] as const,
    detail: (planId: string) => ['plans', 'detail', planId] as const,
  },
  policySlots: {
    list: () => ['policy-slots', 'list'] as const,
  },
  // Each Task kind's deployment-wide common settings; no record involved.
  taskSettings: {
    detail: (operation: string) => ['task-settings', operation] as const,
  },
  preferences: {
    detail: () => ['preferences'] as const,
  },
  upstreamProxy: {
    detail: () => ['upstream-proxy'] as const,
  },
  classifierTags: {
    list: () => ['classifier-tags', 'list'] as const,
  },
  // A collection's detail is keyed by its window: the titles (first 20, newest
  // first) and each random-cover page are separate entries.
  collections: {
    all: () => ['collections'] as const,
    list: () => ['collections', 'list'] as const,
    detail: (id: string, window: CollectionWindow) => ['collections', 'detail', id, window] as const,
    // The detail page's chunked list: one entry per collection and view sort.
    pages: (id: string, sort: string, direction: string) => ['collections', 'pages', id, sort, direction] as const,
  },
  works: {
    all: () => ['works'] as const,
    // The listing: one numbered page, keyed by the whole filter identity plus
    // the page and its size, so every page of every filter is its own entry.
    listPrefix: () => ['works', 'list'] as const,
    list: (filters: ListWorksParams) => ['works', 'list', filters] as const,
    detail: (rj: string) => ['works', 'detail', rj] as const,
    detailPrefix: () => ['works', 'detail'] as const,
    contentPrefix: () => ['works', 'content'] as const,
    facetsPrefix: () => ['works', 'facets'] as const,
    // One work's file manifest: the detail page's deck, bay and leaf read it.
    // The copy's tree identity is part of the content's own response, so the
    // work number alone addresses it.
    content: (rj: string) => ['works', 'content', rj] as const,
    // The account's own tag vocabulary, shared by every work's personal section.
    userTags: () => ['works', 'user-tags'] as const,
    // One facet list per kind and listing scope (the wall's other filters).
    facets: (kind: WorkFacetKind, scope: WorksScope) => ['works', 'facets', kind, scope] as const,
    // Both tag vocabularies, under the facets prefix so every facet sweep
    // covers them; their own prefix lets a user-tag write refresh only them.
    tagFacetsPrefix: () => ['works', 'facets', 'tag'] as const,
    tagFacets: (scope: WorksScope) => ['works', 'facets', 'tag', scope] as const,
  },
  // Every mutable workset resource is keyed by (workset, operation): the same
  // workset id must never address two operations' drafts, sessions or
  // revisions through one cache entry.
  worksets: {
    all: () => ['worksets'] as const,
    listPrefix: () => ['worksets', 'list'] as const,
    list: (libraryId: string | null) => ['worksets', 'list', libraryId ?? ''] as const,
    // All task pages share the library's one current folder batch.
    currentsPrefix: () => ['worksets', 'current'] as const,
    currentPrefix: (libraryId: string) => ['worksets', 'current', libraryId] as const,
    detailsPrefix: () => ['worksets', 'detail'] as const,
    revisionsRecordPrefix: (worksetId: string) => ['worksets', 'revisions', worksetId] as const,
    current: (libraryId: string, _operation?: string) =>
      ['worksets', 'current', libraryId] as const,
    detail: (worksetId: string) => ['worksets', 'detail', worksetId] as const,
    operationPrefix: (worksetId: string) => ['worksets', 'operation', worksetId] as const,
    operation: (worksetId: string, operation: string) =>
      ['worksets', 'operation', worksetId, operation] as const,
    draft: (worksetId: string, operation: string) =>
      ['worksets', 'draft', worksetId, operation] as const,
    revisionsPrefix: (worksetId: string, operation: string) =>
      ['worksets', 'revisions', worksetId, operation] as const,
    revision: (worksetId: string, operation: string, planId: string) =>
      ['worksets', 'revisions', worksetId, operation, planId] as const,
    generation: (worksetId: string, operation: string, generationId: string) =>
      ['worksets', 'planning-sessions', worksetId, operation, generationId] as const,
    executionsPrefix: (worksetId: string, operation: string) =>
      ['worksets', 'executions', worksetId, operation] as const,
    execution: (worksetId: string, operation: string, executionId: string) =>
      ['worksets', 'executions', worksetId, operation, executionId] as const,
    // A metadata execution's cover deliveries live under that execution, so
    // the execution sweeps refresh them too.
    covers: (worksetId: string, executionId: string) =>
      ['worksets', 'executions', worksetId, 'metadata_refresh', executionId, 'covers'] as const,
  },
} as const
