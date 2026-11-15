import type { Page } from '@playwright/test'
import { expect } from '@playwright/test'

/**
 * DESIGN.md §6: text that can hold CJK is never below 12px. Walks the visible
 * text nodes containing Han/Hiragana/Katakana and fails listing every parent
 * whose computed font-size is smaller.
 */
export async function expectCjkFloor(page: Page, floor = 12): Promise<void> {
  const offenders = await page.evaluate((min) => {
    const cjk = /[\p{Script=Han}\p{Script=Hiragana}\p{Script=Katakana}]/u
    const found = new Set<string>()
    const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT)
    for (let node = walker.nextNode(); node; node = walker.nextNode()) {
      const el = node.parentElement
      if (!el || !cjk.test(node.textContent ?? '') || ['SCRIPT', 'STYLE'].includes(el.tagName))
        continue
      const box = el.getBoundingClientRect()
      const style = getComputedStyle(el)
      if (!box.width || !box.height || style.visibility === 'hidden' || style.display === 'none')
        continue
      const size = Number.parseFloat(style.fontSize)
      if (size < min)
        found.add(`${size}px <${el.tagName.toLowerCase()} class="${el.className}"> ${(node.textContent ?? '').trim().slice(0, 30)}`)
    }
    return [...found]
  }, floor)
  expect(offenders, `CJK text below ${floor}px:\n${offenders.join('\n')}`).toEqual([])
}
