import type { WireSchemas } from './wire.gen'
import { afterEach, expect, it, vi } from 'vitest'
import { wireExecution, wireGeneration, wireJson, wireRevision, wireSSE } from '@/test/wire-fixtures'
import { ApiClient } from './client'
import { validateWire, WireContractError } from './wire-validation'

const client = () => new ApiClient({ baseUrl: '/api/v1', token: 'test' })
afterEach(() => vi.unstubAllGlobals())

function membersFixture(): WireSchemas['conversion_v1_executionMembersViewResponse'] {
  const { components: _components, ...base } = wireExecution()
  return {
    ...base,
    members: {
      counts: {
        pending: 0,
        running: 0,
        succeeded: 0,
        failed: 0,
        canceled: 0,
      },
      attention: [],
      attention_total: 0,
    },
  }
}

it('rejects an otherwise valid members view at a full-view call site', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => wireJson('conversion_v1_executionMembersViewResponse', membersFixture())))
  await expect(client().getExecution('w', 'conversion', 'e')).rejects.toBeInstanceOf(WireContractError)
  await expect(client().getExecution('w', 'conversion', 'e', undefined, { view: 'members' })).resolves.toHaveProperty('members')
})

it('rejects the full view at a members-view call site', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => wireJson('conversion_v1_executionFullViewResponse', wireExecution())))
  await expect(client().getExecution('w', 'conversion', 'e', undefined, { view: 'members' })).rejects.toBeInstanceOf(WireContractError)
})

it('binds a schema ID to its exact generated output type', async () => {
  const member = await validateWire('conversion_v1_executionMembersViewResponse', membersFixture())
  expect(member.members.attention_total).toBe(0)
  // @ts-expect-error A members payload has no components, even though another execution schema does.
  expect(member.components).toBeUndefined()
})

it('rejects a value outside a schema enum, so a new backend value cannot pass unseen', async () => {
  const library: WireSchemas['libraryResponse'] = {
    id: 'l',
    name: 'music',
    root_path: '/music',
    storage_class: 'ssd',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    last_scan_at: null,
    last_scan_status: '',
    last_scan_error: '',
  }
  await expect(validateWire('libraryResponse', library)).resolves.toEqual(library)
  await expect(validateWire('libraryResponse', { ...library, storage_class: 'nvme' })).rejects.toBeInstanceOf(WireContractError)
})

it('rejects an undeclared HTTP success status with a valid body', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => wireJson('healthResponse', { ok: true, version: 'dev', workset_max_members: 10000 }, {}, 201)))
  await expect(client().getHealth()).rejects.toThrow('unmapped HTTP status 201')
})

it.each(['surprise', 'toString'])('rejects undeclared SSE event %s', async (event) => {
  // Deliberately invalid protocol fixture: it must bypass the positive typed factory.
  vi.stubGlobal('fetch', vi.fn(async () => new Response(`event: ${event}\ndata: {}\n\n`, {
    headers: { 'Content-Type': 'text/event-stream' },
  })))
  await expect(client().streamMediaFactsEvents(new AbortController().signal).next()).rejects.toThrow(`unmapped SSE event ${event}`)
})

it('rejects an undeclared SSE HTTP status before parsing its valid payload', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => {
    const response = wireSSE('media', [
      { type: 'changed', data: {} },
    ])
    return new Response(response.body, { status: 201, headers: response.headers })
  }))
  await expect(client().streamMediaFactsEvents(new AbortController().signal).next()).rejects.toThrow('unmapped SSE HTTP status 201')
})

it('does not swallow error-envelope schema failures in the proxy JSON fallback', async () => {
  // Deliberately malformed envelope, rather than a valid mock response.
  vi.stubGlobal('fetch', vi.fn(async () => new Response('{"code":123,"message":"bad"}', { status: 404 })))
  await expect(client().getLibrary('missing')).rejects.toBeInstanceOf(WireContractError)
})

it('validates request encoding before fetch', async () => {
  const fetch = vi.fn()
  vi.stubGlobal('fetch', fetch)
  // @ts-expect-error The generated request also refuses this extra field statically.
  await expect(client().createLibrary({ name: 'x', root_path: '/x', typo: true })).rejects.toBeInstanceOf(WireContractError)
  expect(fetch).not.toHaveBeenCalled()
})

it('rejects null for response arrays', async () => {
  await expect(validateWire('libraryListResponse', { libraries: null })).rejects.toBeInstanceOf(WireContractError)
  vi.stubGlobal('fetch', vi.fn(async () => new Response('{"libraries":null}', {
    headers: { 'Content-Type': 'application/json' },
  })))
  await expect(client().listLibraries()).rejects.toBeInstanceOf(WireContractError)
})

it('keeps FileTuple PascalCase keys and requires operation_type on the snapshot', async () => {
  const component: WireSchemas['reconcile_ComponentOutcome'] = {
    component_id: 'c',
    partition: 'matched',
    status: 'ok',
    lanes: [],
    variant_decisions: [],
    operations: [],
    projected_inventory: [],
    files: [
      { Path: 'a', Size: 1, Mtime: 2 },
    ],
  }
  await expect(validateWire('reconcile_ComponentOutcome', component)).resolves.toEqual(component)
  const { operation_type: _operation, ...snapshot } = wireGeneration()
  await expect(validateWire('generationViewResponse', snapshot)).rejects.toBeInstanceOf(WireContractError)
  await expect(validateWire('generationViewResponse', wireGeneration())).resolves.toEqual(wireGeneration())
})

it('requires the frozen policy and summary in a plan payload', async () => {
  const revision = wireRevision()
  await expect(validateWire('conversion_v1_revisionResponse', revision)).resolves.toEqual(revision)
  for (const field of ['policy', 'summary']) {
    const broken = {
      ...revision,
      task: {
        ...revision.task,
        payload: { ...revision.task.payload, [field]: null },
      },
    }
    await expect(validateWire('conversion_v1_revisionResponse', broken)).rejects.toBeInstanceOf(WireContractError)
  }
})

it('rejects malformed payloads of a declared SSE event', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => new Response('event: changed\ndata: {"work_rj":42}\n\n')))
  await expect(client().streamMediaFactsEvents(new AbortController().signal).next()).rejects.toBeInstanceOf(WireContractError)
})
