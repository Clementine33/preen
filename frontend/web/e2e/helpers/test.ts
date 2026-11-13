import { test as base } from '@playwright/test'

export { expect } from '@playwright/test'

/**
 * `test` for the real-stack specs. The fixture audio is dummy bytes, so every
 * Media Facts Pass ends with a sticky "无法测量" toast that would cover the
 * header controls; the stack specs never assert on toasts, so let clicks pass
 * through them.
 */
export const test = base.extend({
  page: async ({ page }, use) => {
    await page.addInitScript(() => {
      const style = document.createElement('style')
      style.textContent = '[data-testid="task-toast"]{pointer-events:none!important}'
      document.addEventListener('DOMContentLoaded', () => document.head.append(style))
    })
    await use(page)
  },
})
