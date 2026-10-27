import { defineConfig, devices } from '@playwright/test'

// Real browser layout and input checks need Vite, without a backend process.
const selectTestURL = (globalThis as { process?: { env?: Record<string, string | undefined> } }).process?.env?.PREEN_SELECT_TEST_URL
const baseURL = selectTestURL || 'http://127.0.0.1:5174'

export default defineConfig({
  testDir: './e2e',
  testMatch: ['unit-fields.spec.ts', 'task-toast.spec.ts'],
  workers: 1,
  timeout: 60_000,
  expect: { timeout: 15_000 },
  use: { baseURL, trace: 'retain-on-failure' },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
  webServer: selectTestURL
    ? undefined
    : {
        command: 'pnpm exec vite --port 5174 --strictPort --host 127.0.0.1',
        url: baseURL,
        reuseExistingServer: false,
      },
})
