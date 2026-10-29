import { expect, test } from '@playwright/test'

for (const width of [393, 1440]) {
  test(`5000 members remain scrollable, selectable and reachable at ${width}px`, async ({ page }) => {
    const errors: string[] = []
    page.on('pageerror', error => errors.push(error.message))
    await page.setViewportSize({ width, height: 852 })
    await page.route('**/__member-list', route => route.fulfill({
      contentType: 'text/html',
      body: '<html><head><meta name="viewport" content="width=device-width,initial-scale=1"></head><body><div id="app"></div><script type="module" src="/e2e/member-list-harness.js"></script></body></html>',
    }))
    await page.goto('/__member-list')
    const scroller = page.getByTestId('member-scroller')
    const rows = page.getByTestId('member-row')
    await expect(rows.first()).toBeVisible()
    expect(await rows.count()).toBeLessThan(40)
    await expect(rows.first()).toHaveAttribute('aria-posinset', '1')
    await expect(rows.first()).toHaveAttribute('aria-setsize', '5000')
    expect(await scroller.evaluate(el => el.clientHeight)).toBeGreaterThan(300)
    expect(await scroller.evaluate(el => el.scrollWidth <= el.clientWidth)).toBe(true)

    // Native Tab and Space retain their meaning on a selected member's checkbox.
    const firstCheckbox = rows.first().getByRole('checkbox')
    await firstCheckbox.focus()
    await page.keyboard.press('Space')
    await expect(firstCheckbox).toBeChecked()
    await page.keyboard.press('Tab')
    await expect(rows.first().getByTestId('member-open')).toBeFocused()

    await scroller.evaluate((el) => { el.scrollTop = 80000 })
    await expect.poll(async () => Number(await rows.first().getAttribute('data-index'))).toBeGreaterThan(100)
    expect(await rows.count()).toBeLessThan(40)
    const open = rows.nth(10).getByTestId('member-open')
    await open.scrollIntoViewIfNeeded()
    const name = (await open.textContent())!.trim()
    const checkbox = rows.filter({ hasText: name }).getByRole('checkbox')
    await checkbox.check()
    const offset = await scroller.evaluate(el => el.scrollTop)
    await open.click()
    await page.getByRole('button', { name: '返回列表' }).click()
    await expect(rows.filter({ hasText: name }).getByRole('checkbox')).toBeChecked()
    await expect.poll(async () => Math.abs(await scroller.evaluate(el => el.scrollTop) - offset)).toBeLessThan(2)
    await checkbox.focus()
    await expect(checkbox).toBeFocused()
    await scroller.evaluate((el) => { el.scrollTop = el.scrollHeight })
    await expect(rows.last()).toContainText('Folder 4999')
    await expect(rows.last()).toHaveAttribute('aria-posinset', '5000')

    // Filtering a deep window must expose its result at the start of the list.
    await page.getByRole('searchbox').fill('Folder 4999')
    await expect(rows).toHaveCount(1)
    await expect(rows.first()).toBeVisible()
    await expect(rows.first()).toContainText('已排除')
    await expect(rows.first()).toHaveAttribute('aria-posinset', '1')
    await expect(rows.first()).toHaveAttribute('aria-setsize', '1')
    await expect(page.getByTestId('hidden-selection')).toBeVisible()
    await page.getByRole('button', { name: '已排除', exact: true }).click()
    await expect(rows).toHaveCount(1)
    await page.getByRole('searchbox').fill('')
    await page.getByRole('button', { name: '全部', exact: true }).click()
    await expect(rows.first()).toHaveAttribute('data-index', '0')
    await expect(firstCheckbox).toBeChecked()
    await expect(page.getByRole('list', { name: '工作集成员与转换结论' })).toBeVisible()
    // Tab crosses virtual windows; actions after the initial DOM slice remain reachable.
    await firstCheckbox.focus()
    for (let index = 0; index < 121; index++)
      await page.keyboard.press('Tab')
    await expect(page.locator(':focus')).toHaveText('Folder 30')
    expect(await scroller.evaluate(el => el.scrollTop)).toBeGreaterThan(0)
    expect(errors).toEqual([])
  })
}
