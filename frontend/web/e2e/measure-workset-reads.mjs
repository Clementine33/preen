// Opt-in companion to TestMeasureWorksetLifecycle. Every API body comes from
// that test's live production HTTP handler and SQLite lifecycle, without mocks.
import assert from 'node:assert/strict'
import { fileURLToPath } from 'node:url'
import { chromium } from '@playwright/test'
import { createServer } from 'vite'

async function main() {
  const [phase, backend, library, workset, plan, member, execution] = process.argv.slice(2)
  assert(['revision', 'execution', 'execution-stream'].includes(phase), 'expected revision, execution or execution-stream phase')
  const streaming = phase === 'execution-stream'
  process.env.PREEN_BACKEND_URL = backend
  process.env.PREEN_VUE_DEVTOOLS = '0'
  const root = fileURLToPath(new URL('..', import.meta.url))
  const vite = await createServer({
    root,
    server: { host: '127.0.0.1', port: 5189, strictPort: true },
  })
  await vite.listen()
  const browser = await chromium.launch({
    args: ['--enable-precise-memory-info', '--remote-debugging-port=9229'],
  })
  console.log(JSON.stringify({
    browser: browser.version(),
    phase,
    plan,
    url: `http://127.0.0.1:5189/worksets/${library}/conversion`,
  }))
  const operation = `/api/v1/worksets/${workset}/operations/conversion`
  const revisionPath = `${operation}/revisions/${plan}`
  try {
    for (const width of [393, 1440]) {
      const page = await browser.newPage({
        viewport: { width, height: 852 },
      })
      page.setDefaultTimeout(120_000)
      const errors = []
      page.on('pageerror', error => errors.push(error.message))
      await page.addInitScript(() => {
        window.measureReads = []
        const nativeFetch = window.fetch.bind(window)
        window.fetch = async (...args) => {
          const start = performance.now()
          const response = await nativeFetch(...args)
          const url = typeof args[0] === 'string'
            ? args[0]
            : args[0].url
          const nativeText = response.text.bind(response)
          response.json = async () => {
            const text = await nativeText()
            const received = performance.now()
            const body = JSON.parse(text)
            const parsed = performance.now()
            window.measureReads.push({
              url,
              start,
              received,
              parsed,
              parse_ms: parsed - received,
              bytes: new TextEncoder().encode(text).length,
              handler: response.headers.get('server-timing'),
              components: body.task?.payload?.components?.length,
              status: body.status,
            })
            return body
          }
          return response
        }
      })
      // Warm code/font delivery once, then start a fresh document/query cache.
      // The timed API reads below still execute against the live handler.
      const executionPage = phase.startsWith('execution') && process.env.PREEN_MEASURE_SNAPSHOT_LIST !== '1'
      const listURL = `http://127.0.0.1:5189/worksets/${library}/conversion${executionPage ? '/execution' : ''}`
      await page.goto(listURL)
      await page.getByTestId(executionPage ? 'execution-panel' : 'member-row').first().waitFor()
      await page.evaluate(() => document.fonts.ready)
      if (process.env.PREEN_MEASURE_INSPECT === '1' && phase === 'revision' && width === 393) {
        console.log('inspect ready: agent-browser --cdp 9229 snapshot -i; continuing in 30 seconds')
        await new Promise(resolve => setTimeout(resolve, 30_000))
      }
      await page.reload()
      if (phase === 'revision') {
        await page.waitForFunction(path => window.measureReads.some(read => read.url === path), revisionPath)
        await page.waitForFunction(() => [
          ...document.querySelectorAll('[data-testid="member-row"]'),
        ].some(row => row.textContent.includes('有变化')))
        const list = await page.evaluate((path) => {
          const read = window.measureReads.find(read => read.url === path)
          return {
            ...read,
            ready: performance.now(),
            heap_bytes: performance.memory.usedJSHeapSize,
            rows: document.querySelectorAll('[data-testid="member-row"]').length,
          }
        }, revisionPath)
        console.log(JSON.stringify({
          phase,
          width,
          list,
          navigation_ms: list.ready,
          revision_to_render_ms: list.ready - list.start,
          requests: await page.evaluate(() => window.measureReads),
        }))
        // Retain the section 6 scroll workload: 120 rAF jumps from top to last row.
        const scroll = await page.getByTestId('member-scroller').evaluate(async (el) => {
          const frames = []
          let previous = performance.now()
          let maxRows = 0
          for (let index = 0; index < 120; index++) {
            el.scrollTop = (el.scrollHeight - el.clientHeight) * index / 119
            await new Promise(requestAnimationFrame)
            const now = performance.now()
            frames.push(now - previous)
            previous = now
            maxRows = Math.max(maxRows, document.querySelectorAll('[data-testid="member-row"]').length)
          }
          frames.sort((a, b) => a - b)
          return {
            max_rows: maxRows,
            frame_p95_ms: frames[113],
            frame_max_ms: frames[119],
            over_50ms: frames.filter(frame => frame > 50).length,
          }
        })
        await page.getByTestId('member-scroller').evaluate((el) => { el.scrollTop = 0 })
        await page.waitForFunction(() => document.querySelector('[data-testid="member-row"]')?.getAttribute('aria-posinset') === '1')
        const before = await page.evaluate(() => ({
          time: performance.now(),
          reads: window.measureReads.length,
        }))
        // The largest member has the first root when all component counts tie.
        const link = page.getByTestId('member-open').first()
        assert.equal(await link.count(), 1, 'largest member must be a visible real member link')
        await link.click()
        await page.waitForURL(`**/${member}`)
        await page.getByTestId('member-review').waitFor({ state: 'visible' })
        const review = await page.evaluate(({ before, path }) => ({
          elapsed_ms: performance.now() - before.time,
          heap_bytes: performance.memory.usedJSHeapSize,
          // The harness has no media-facts broker; its 503 retry is background noise.
          requests: window.measureReads.slice(before.reads).filter(read => !read.url.includes('/inventory/media-facts/events')),
          decisions: document.querySelectorAll('[data-testid="component-decision"]').length,
          path,
        }), { before, path: `${revisionPath}/members/${member}` })
        assert.equal(review.requests.length, 1, `opening one member must request only its review: ${review.requests.map(read => read.url).join(', ')}`)
        assert.equal(review.requests[0].url, review.path)
        assert.equal(review.requests[0].components, 1, 'the review must contain only the selected member component')
        console.log(JSON.stringify({ phase, width, review, scroll }))
      }
      else {
        await page.getByTestId(executionPage ? 'execution-panel' : 'member-row').first().waitFor()
        if (executionPage) {
          // Slim execution page and member page, each reloaded from a fresh Vue
          // Query cache. Streaming means the session is still running before and
          // after each sample; the member is in scope and has frozen keep decisions.
          const executionPath = `${operation}/executions/${execution}`
          const sessionStatus = async () => {
            const body = await (await fetch(`${backend}${executionPath}?view=members`)).json()
            return `${body.status} ${body.completed_components}/${body.total_components}`
          }
          const sample = async (kind, path, ready, statusTestId) => {
            const url = `http://127.0.0.1:5189${path}`
            await page.goto(url)
            await page.locator(ready).first().waitFor()
            await page.evaluate(() => document.fonts.ready)
            const before = await sessionStatus()
            await page.reload()
            await page.locator(ready).first().waitFor()
            const visible = await page.evaluate(() => performance.now())
            const after = await sessionStatus()
            const ui = await page.getByTestId(statusTestId).first().textContent()
            const reads = await page.evaluate(() => window.measureReads)
            let kept
            if (kind === 'member') {
              const body = await (await fetch(`${backend}${executionPath}?member_id=${member}`)).json()
              kept = body.components.reduce((sum, component) => sum + component.kept_decisions.length, 0)
              if (process.env.PREEN_MEASURE_KEPT_SNAPSHOT === '1')
                assert(kept > 0, 'the sampled member must have real keep decisions')
            }
            if (streaming)
              assert(before.startsWith('running') && after.startsWith('running'), `${kind}: session must be running around the sample (${before}/${after})`)
            console.log(JSON.stringify({
              phase,
              width,
              kind,
              visible_ms: visible,
              session_before: before,
              session_after: after,
              ui_status: ui.trim(),
              kept,
              requests: reads.map(({ url, bytes, handler, parse_ms, status }) => ({
                url,
                bytes,
                handler,
                parse_ms,
                status,
              })),
            }))
          }
          await sample('panel', `/worksets/${library}/conversion/execution`, '[data-testid="execution-count-pending"]', 'execution-status')
          await sample('member', `/worksets/${library}/conversion/${member}/execution`, '[data-testid="execution-component"]', 'execution-member-status')
          if (streaming) {
            assert.deepEqual(errors, [])
            await page.close()
            continue
          }
        }
        // Two separate real SSE connections to the same persisted execution;
        // reconnect deliberately sends no replay id, matching production semantics.
        console.log(JSON.stringify({
          phase,
          width,
          execution_page: executionPage,
          execution_page_ms: await page.evaluate(() => performance.now()),
          requests: await page.evaluate(() => window.measureReads),
        }))
        for (const connection of ['first', 'reconnect']) {
          const snapshot = await page.evaluate(async (path) => {
            const start = performance.now()
            const response = await fetch(path)
            const reader = response.body.getReader()
            const decoder = new TextDecoder()
            const chunks = []
            let tail = ''
            let complete = false
            let bytes = 0
            let firstByte
            while (!complete) {
              const result = await reader.read()
              if (result.done)
                throw new Error('snapshot missing')
              firstByte ??= performance.now()
              bytes += result.value.length
              const chunk = decoder.decode(result.value, { stream: true })
              complete = (tail + chunk).includes('\n\n')
              tail = chunk.slice(-1)
              chunks.push(chunk)
            }
            const received = performance.now()
            const text = chunks.join('')
            const frame = text.slice(0, text.indexOf('\n\n'))
            assertSnapshot(frame)
            function assertSnapshot(value) {
              if (!value.startsWith('event: execution_snapshot\n'))
                throw new Error('first event is not snapshot')
            }
            const payload = frame.slice(frame.indexOf('data: ') + 6)
            const parseStart = performance.now()
            const body = JSON.parse(payload)
            const parsed = performance.now()
            await reader.cancel()
            return {
              handler: response.headers.get('server-timing'),
              payload_bytes: new TextEncoder().encode(payload).length,
              frame_bytes: new TextEncoder().encode(`${frame}\n\n`).length,
              received_bytes: bytes,
              first_byte_ms: firstByte - start,
              receive_ms: received - firstByte,
              fetch_ms: received - start,
              parse_ms: parsed - parseStart,
              total_ms: parsed - start,
              members: Object.values(body.members.counts).reduce((sum, n) => sum + n, 0),
              attention: body.members.attention.length,
              heap_bytes: performance.memory.usedJSHeapSize,
            }
          }, `${operation}/executions/${execution}/events?view=members`)
          console.log(JSON.stringify({ phase, width, connection, snapshot }))
        }
      }
      assert.deepEqual(errors, [])
      await page.close()
    }
  }
  finally {
    await browser.close()
    await vite.close()
  }
}
main().catch((error) => { console.error(error); process.exitCode = 1 })
