import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiClient, ApiError } from './client'
import type { ApiConfig } from './types'

function createClient(config: Partial<ApiConfig> = {}): ApiClient {
  return new ApiClient({ baseUrl: config.baseUrl ?? 'http://example.test/api/v1', token: config.token ?? null })
}

function okJson(body: unknown, headers: Record<string, string> = {}, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json', ...headers } })
}

// Typed fetch mock factory so mock.calls elements keep their RequestInit type.
function mockFetch(
  handler: (input: RequestInfo | URL, init?: RequestInit) => Response | Promise<Response>,
) {
  const fn = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    return await handler(input, init)
  })
  vi.stubGlobal('fetch', fn)
  return fn
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('ApiClient', () => {
  it.each([1, 2, 3, undefined] as const)('sends the selected age category %s in the works request', async (age) => {
    const fetchMock = mockFetch(async () => okJson({ works: [] }))

    await createClient().listWorks({ age_category: age })

    const query = new URL(String(fetchMock.mock.calls[0]![0])).searchParams
    expect(query.get('age_category')).toBe(age === undefined ? null : String(age))
  })

  it('sends the collection window, and only the detail route takes a direction', async () => {
    const fetchMock = mockFetch(async () => okJson({ collections: [], works: [] }))

    await createClient().listCollections({ limit: 200 })
    await createClient().getCollection('favorites', { limit: 20, direction: 'desc' })
    await createClient().getCollection('a/b')

    const urls = fetchMock.mock.calls.map((call) => new URL(String(call[0])))
    expect(urls[0]!.pathname.endsWith('/collections')).toBe(true)
    expect(urls[0]!.search).toBe('?limit=200')
    expect(urls[1]!.pathname.endsWith('/collections/favorites')).toBe(true)
    expect(urls[1]!.searchParams.get('direction')).toBe('desc')
    expect(urls[2]!.pathname.endsWith('/collections/a%2Fb')).toBe(true)
    expect(urls[2]!.search).toBe('')
  })

  it('sends the combined tag filter and its mode in the works request', async () => {
    const fetchMock = mockFetch(async () => okJson({ works: [] }))

    await createClient().listWorks({ tag: 's:a|u:b,s:c', tag_match: 'any' })

    const query = new URL(String(fetchMock.mock.calls[0]![0])).searchParams
    expect(query.get('tag')).toBe('s:a|u:b,s:c')
    expect(query.get('tag_match')).toBe('any')
  })

  it('reads and writes a Task kind\'s settings with their ETag', async () => {
    const fetchMock = mockFetch(async (input, init) => {
      expect(String(input)).toContain('/tasks/rename/settings')
      if (init?.method === 'PUT') {
        expect(new Headers(init.headers).get('If-Match')).toBe('"abc"')
        expect(JSON.parse(String(init.body))).toEqual({ settings: { template: '{rj}' } })
        return okJson({ operation_type: 'rename', settings: { template: '{rj}' } }, { ETag: '"def"' })
      }
      return okJson({ operation_type: 'rename', settings: { template: '[{rj}] {title}' } }, { ETag: '"abc"' })
    })

    const client = createClient()
    const read = await client.getTaskSettings('rename')
    expect(read).toEqual({ settings: { template: '[{rj}] {title}' }, etag: '"abc"' })
    const saved = await client.saveTaskSettings('rename', { template: '{rj}' }, read.etag)
    expect(saved).toEqual({ settings: { template: '{rj}' }, etag: '"def"' })
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('passes a non-conversion draft through without conversion defaults', async () => {
    // Rename and metadata drafts reject unknown fields, so a save of what was
    // read must not carry conversion's no_effect_tags back.
    const payload = { schema_version: 1, common: { template: '{rj}' }, members: [] }
    mockFetch(async () => okJson({
      workset_id: 'ws-1',
      operation_type: 'rename',
      version: 2,
      task: { kind: 'rename', schema_version: 1, payload },
      updated_at: '',
    }))

    const draft = await createClient().getOperationDraft('ws-1', 'rename')
    expect(draft.document).toEqual(payload)
  })

  it('wires the query-owned AbortSignal through to fetch', async () => {
    const captured: AbortSignal[] = []
    const fetchMock = mockFetch((_input, init) => {
      captured.push(init?.signal as AbortSignal)
      // Stay pending until the internal signal aborts, so the caller-side
      // abort reaches the fetch signal while the request is in flight.
      return new Promise<Response>((_resolve, reject) => {
        init?.signal?.addEventListener('abort', () =>
          reject(new DOMException('Aborted', 'AbortError')),
          { once: true },
        )
      })
    })

    const client = createClient()
    const controller = new AbortController()

    const promise = client.listLibraries(controller.signal)
    controller.abort()

    await expect(promise).rejects.toMatchObject({ name: 'AbortError' })
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(captured[0]).toBeInstanceOf(AbortSignal)
    // The abort propagated to the fetch signal that was actually handed out:
    expect(captured[0].aborted).toBe(true)
  })

  it('passes the signal through the directory listing and a member tree', async () => {
    const captured: (AbortSignal | undefined)[] = []
    const fetchMock = mockFetch((_input, init) => {
      captured.push(init?.signal ?? undefined)
      return new Promise<Response>((_resolve, reject) => {
        init?.signal?.addEventListener('abort', () =>
          reject(new DOMException('Aborted', 'AbortError')),
          { once: true },
        )
      })
    })

    const client = createClient()
    const controller = new AbortController()

    const calls = [
      client.listDirs('lib-1', controller.signal),
      client.getMemberTree('lib-1', 'albumA', controller.signal),
    ]
    controller.abort()

    for (const call of calls) {
      await expect(call).rejects.toMatchObject({ name: 'AbortError' })
    }
    expect(fetchMock).toHaveBeenCalledTimes(2)
    for (const signal of captured) {
      expect(signal).toBeInstanceOf(AbortSignal)
      expect((signal as AbortSignal).aborted).toBe(true)
    }
  })

  it('sends If-Match and Idempotency-Key headers on workset writes', async () => {
    const fetchMock = mockFetch(async (input, init) => {
      const path = String(input)
      const headers = new Headers(init?.headers)
      if (path.endsWith('/draft')) {
        expect(headers.get('If-Match')).toBe('3')
        expect(JSON.parse(String(init?.body)).apply_common_settings).toBeUndefined()
        return okJson({ workset_id: 'ws-1', version: 4, planning_state: 'planned', members: [] })
      }
      if (path.endsWith('/revisions')) {
        expect(path).toContain('/operations/conversion/')
        expect(headers.get('Idempotency-Key')).toBe('key-1')
        expect(headers.get('If-Match')).toBe('3')
        return okJson({ created: true, generation: { generation_id: 'gen-1', status: 'queued' } })
      }
      if (path.endsWith('/operations/conversion/current') && init?.method === 'PUT') {
        expect(headers.get('Idempotency-Key')).toBe('create-key')
        return okJson({ workset: { workset_id: 'ws-1', members: [] }, created: true, recorded: 1, skipped: [] })
      }
      return okJson({})
    })

    const client = createClient()
    await client.saveOperationDraft(
      'ws-1',
      'conversion',
      {
        schema_version: 1,
        no_effect_tags: [],
        matched: {},
        unmatched: {},
        members: [],
      },
      3,
    )
    await client.startGeneration('ws-1', 'conversion', 3, 'key-1')
    await client.createCurrentRecord('lib-1', 'conversion', { folder_paths: ['albumA'] }, 'create-key')
    expect(fetchMock).toHaveBeenCalledTimes(3)
  })

  it('always applies a management signal even when none is provided', async () => {
    const fetchMock = mockFetch(async () => okJson({ libraries: [] }))

    await createClient().listLibraries()

    expect(fetchMock).toHaveBeenCalledTimes(1)
    // The request gets an internal timeout signal even without a caller one.
    expect(fetchMock.mock.calls[0][1]?.signal).toBeInstanceOf(AbortSignal)
  })

  it('times out a request that never settles', async () => {
    vi.useFakeTimers()
    try {
      const fetchMock = mockFetch(async (_input, init) => {
        await new Promise((_resolve, reject) => {
          init?.signal?.addEventListener('abort', () =>
            reject(new DOMException('请求超时', 'TimeoutError')),
          )
        })
        return okJson({ libraries: [] })
      })

      const promise = createClient().listLibraries()
      // Attach the rejection handler before the timer fires so the timed-out
      // request is never flagged as an unhandled rejection.
      const expectation = expect(promise).rejects.toMatchObject({ name: 'TimeoutError' })
      await vi.advanceTimersByTimeAsync(15_000)

      await expectation
      expect(fetchMock).toHaveBeenCalledTimes(1)
    } finally {
      vi.useRealTimers()
    }
  })

  it('surfaces fetch aborts without wrapping them as ApiError', async () => {
    const abortError = new DOMException('The user aborted a request.', 'AbortError')
    const fetchMock = mockFetch(async () => {
      throw abortError
    })

    const client = createClient()
    const controller = new AbortController()
    const promise = client.listLibraries(controller.signal)
    controller.abort()

    await expect(promise).rejects.toBe(abortError)
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('parses the JSON envelope and wraps failures as ApiError', async () => {
    const fetchMock = mockFetch(async () => {
      return new Response(JSON.stringify({ code: 'LIBRARY_NOT_FOUND', message: 'no such library' }), {
        status: 404,
        headers: { 'Content-Type': 'application/json' },
      })
    })

    const error = await createClient().getLibrary('missing').catch((e: unknown) => e)

    expect(error).toBeInstanceOf(ApiError)
    expect(error).toMatchObject({ code: 'LIBRARY_NOT_FOUND', status: 404 })
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('treats a 204 response as undefined', async () => {
    const fetchMock = mockFetch(async () => new Response(null, { status: 204 }))

    await expect(createClient().deleteLibrary('lib-1')).resolves.toBeUndefined()
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })
})

describe('work content routes', () => {
  const ref = { rj: 'RJ01000001', copyId: 'c-1', relPath: '表紙/cover main.jpg' }

  it('puts the token in an element-loaded file address, each segment escaped', () => {
    expect(createClient({ token: 't&1' }).workFileUrl(ref)).toBe(
      'http://example.test/api/v1/works/RJ01000001/content/c-1/%E8%A1%A8%E7%B4%99/cover%20main.jpg?access_token=t%261',
    )
    expect(createClient().workFileUrl(ref)).not.toContain('access_token')
  })

  it('versions original images without changing the file route or token', () => {
    const url = new URL(createClient({ token: 'secret' }).workFileUrl(ref, 'v&2'))
    expect(url.searchParams.get('v')).toBe('v&2')
    expect(url.searchParams.get('access_token')).toBe('secret')
    expect(url.pathname).toContain('/content/c-1/')
  })

  it('parses media notifications through the authenticated SSE client', async () => {
    const fetchMock = mockFetch(() => new Response('event: changed\ndata: {}\n\n: keep-alive\n\n', {
      headers: { 'Content-Type': 'text/event-stream' },
    }))
    const signal = new AbortController().signal
    const events = []
    for await (const event of createClient({ token: 'secret' }).streamMediaFactsEvents(signal)) events.push(event)
    expect(events).toEqual([{ type: 'changed', data: {} }])
    const [url, init] = fetchMock.mock.calls[0]
    expect(String(url)).toContain('/inventory/media-facts/events')
    expect(new Headers(init?.headers).get('Authorization')).toBe('Bearer secret')
    expect(init?.signal).toBe(signal)
  })

  it('decodes text by the charset the server declared', async () => {
    // 「こんにちは」 in Shift_JIS.
    const sjis = new Uint8Array([0x82, 0xb1, 0x82, 0xf1, 0x82, 0xc9, 0x82, 0xbf, 0x82, 0xcd])
    mockFetch(() => new Response(sjis, { headers: { 'Content-Type': 'text/plain; charset=shift_jis' } }))
    await expect(createClient().getWorkText(ref)).resolves.toBe('こんにちは')
  })

  it('backs off a full thumbnail queue by Retry-After, header-authenticated', async () => {
    vi.useFakeTimers()
    let calls = 0
    const fetchMock = mockFetch(() =>
      ++calls === 1
        ? new Response(null, { status: 503, headers: { 'Retry-After': '2' } })
        : new Response('png', { headers: { 'Content-Type': 'image/png' } }),
    )
    const pending = createClient({ token: 'secret' }).getWorkThumb(ref, 120, 'v1')
    await vi.advanceTimersByTimeAsync(2000)
    const blob = await pending
    vi.useRealTimers()
    expect(await blob.text()).toBe('png')
    const [url, init] = fetchMock.mock.calls[1]
    expect(String(url)).toContain('/thumbs/c-1/')
    expect(String(url)).toContain('?h=120&v=v1')
    expect(String(url)).not.toContain('secret')
    expect(new Headers(init?.headers).get('Authorization')).toBe('Bearer secret')
  })
})

it('normalizes legacy conversion tags at every stored JSON read without losing empty overrides', async () => {
  const legacy = { schema_version: 1, classifier_tags: ['SEなし'], matched: {}, unmatched: {} }
  mockFetch((input) => {
    const url = String(input)
    if (url.endsWith('/policy-slots')) return okJson({ slots: [{ slot: 1, name: 'old', policy: legacy }, { slot: 2, name: 'new', policy: { ...legacy, no_effect_tags: [] } }] })
    if (url.endsWith('/draft')) return okJson({ operation_type: 'conversion', task: { kind: 'conversion', schema_version: 1, payload: { ...legacy, members: [{ member_id: 'one', overrides: { classifier_tags: [] } }, { member_id: 'two' }] } } })
    return okJson({ task: { kind: 'conversion', payload: { policy: legacy } } })
  })
  const client = createClient()
  const draft = await client.getOperationDraft('ws', 'conversion')
  expect(draft.document.no_effect_tags).toEqual(['SEなし'])
  expect(draft.document.members[0]?.overrides).toEqual({ no_effect_tags: [] })
  expect(draft.document.members[1]?.overrides).toBeUndefined()
  expect(draft.document).not.toHaveProperty('classifier_tags')
  const slots = await client.listPolicySlots()
  expect(slots[0]?.policy).toMatchObject({ no_effect_tags: ['SEなし'], other_tags: [] })
  expect(slots[1]?.policy?.no_effect_tags).toEqual([])
  expect((await client.getRevision('ws', 'conversion', 'p')).task.payload.policy).toMatchObject({ no_effect_tags: ['SEなし'], other_tags: [] })
})
