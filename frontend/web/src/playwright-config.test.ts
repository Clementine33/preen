import { afterEach, describe, expect, it, vi } from 'vitest'

describe('playwright stack configuration', () => {
  afterEach(() => {
    vi.unstubAllEnvs()
    vi.resetModules()
  })

  it('does not start a web server when the e2e smoke is disabled', async () => {
    vi.stubEnv('BROWSER_E2E', '')
    vi.resetModules()

    const { default: config } = await import('../playwright.config')

    expect(config.webServer).toBeUndefined()
  })

  it('starts the real stack when the e2e smoke is enabled', async () => {
    vi.stubEnv('BROWSER_E2E', '1')
    vi.resetModules()

    const { default: config } = await import('../playwright.config')

    expect(config.webServer).toMatchObject({
      command: 'node e2e/launch-stack.mjs',
      url: 'http://127.0.0.1:5175',
      env: { PREEN_E2E_WEB_PORT: '5175' },
    })
    expect(config.use?.baseURL).toBe('http://127.0.0.1:5175')
    expect(config.testIgnore).toEqual(['**/unit-fields.spec.ts', '**/task-toast.spec.ts', '**/member-list.spec.ts'])
  })

  it('runs menu components with Vite only', async () => {
    vi.stubEnv('PREEN_SELECT_TEST_URL', '')
    vi.resetModules()

    const { default: config } = await import('../playwright.components.config')

    expect(config.testMatch).toEqual(['unit-fields.spec.ts', 'task-toast.spec.ts', 'member-list.spec.ts'])
    expect(config.webServer).toMatchObject({
      command: 'pnpm exec vite --port 5174 --strictPort --host 127.0.0.1',
      url: 'http://127.0.0.1:5174',
    })
  })
})
