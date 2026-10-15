import { expect, test } from '@playwright/test'

for (const mobile of [false, true]) {
  test(mobile ? 'mobile task notices preserve undo and wrap below safe area' : 'desktop task notices preserve undo at upper right', async ({ page }) => {
    await page.setViewportSize(mobile ? { width: 320, height: 640 } : { width: 1440, height: 900 })
    await page.emulateMedia({ reducedMotion: 'reduce' })
    await page.route('**/__task-toast', (route) => route.fulfill({ contentType: 'text/html', body: `
      <html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover"></head>
      <body><div id="app"></div><script type="module">
        import { createApp, h } from '/node_modules/.vite/deps/vue.js';
        import TaskToastHost from '/src/components/layout/TaskToastHost.vue';
        import ToastHost from '/src/components/layout/ToastHost.vue';
        import { useTaskToast } from '/src/composables/use-task-toast.ts';
        import { useToast } from '/src/composables/use-toast.ts';
        import '/src/style.css';
        const tasks = useTaskToast();
        const edit = useToast();
        createApp({ setup: () => () => [
          h('button', { id: 'enqueue', onclick: () => {
            edit.show({ text: '已从合集移除', action: { label: '撤销', run: () => {
              document.querySelector('#undo-result').textContent = 'restored';
            } } });
            tasks.show({ text: 'Archive · 扫描完成：很长的媒体库名称和结果说明需要完整换行以便查看已完成的扫描结果。'.repeat(3), action: { label: '查看结果', run: () => {} } });
            tasks.show({ text: '规划失败', action: { label: '查看结果', run: () => {} } });
          } }, 'Enqueue'),
          h('span', { id: 'undo-result' }), h(ToastHost), h(TaskToastHost),
        ] }).mount('#app');
      </script></body></html>` }))
    await page.goto('/__task-toast')
    await page.locator('#enqueue').click()
    const task = page.getByTestId('task-toast')
    await expect(task).toBeVisible()
    await expect(page.getByTestId('toast')).toContainText('已从合集移除')
    const box = (await task.boundingBox())!
    const viewport = page.viewportSize()!
    expect(box.y).toBeGreaterThanOrEqual(16)
    expect(box.x).toBeGreaterThanOrEqual(16)
    expect(box.x + box.width).toBeLessThanOrEqual(viewport.width - 15)
    expect(box.height).toBeLessThan(viewport.height / 2)
    if (!mobile) expect(box.x).toBeGreaterThan(viewport.width / 2)
    expect(await task.evaluate(el => el.scrollWidth <= el.clientWidth)).toBe(true)
    await expect(page.getByTestId('task-toast-action')).toBeVisible()
    const contrast = await task.evaluate(el => {
      const style = getComputedStyle(el)
      const luminance = (color: string) => {
        const values = color.match(/[\d.]+/g)!.slice(0, 3).map(Number).map(value => {
          const channel = value / 255
          return channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4
        })
        return values[0]! * 0.2126 + values[1]! * 0.7152 + values[2]! * 0.0722
      }
      const foreground = luminance(style.color)
      const background = luminance(style.backgroundColor)
      return (Math.max(foreground, background) + 0.05) / (Math.min(foreground, background) + 0.05)
    })
    expect(contrast).toBeGreaterThanOrEqual(4.5)
    await page.getByTestId('toast-action').click()
    await expect(page.locator('#undo-result')).toHaveText('restored')
    await expect(task).toContainText('扫描完成')
    await task.hover()
    await page.waitForTimeout(4200)
    await expect(task).toContainText('扫描完成')
    await page.getByTestId('task-toast-action').focus()
    await page.mouse.move(0, 500)
    await page.waitForTimeout(4200)
    await expect(task).toContainText('扫描完成')
    await page.keyboard.press('Escape')
    await expect(task).toContainText('规划失败')
    await page.locator('#enqueue').focus()
    await expect(task).toBeHidden({ timeout: 6000 })
    const motion = await page.evaluate(() => matchMedia('(prefers-reduced-motion: reduce)').matches)
    expect(motion).toBe(true)
  })
}
