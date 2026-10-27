import { expect, test } from '@playwright/test'

/**
 * The shared conversion fields with production CSS, at desktop and under
 * touch emulation: every choice is an inline segmented row, so a pick is one
 * tap and the bitrate box exists only while an encoded format is chosen.
 */
for (const isMobile of [false, true]) {
  test.describe(isMobile ? 'mobile emulation' : 'desktop', () => {
    test.use({
      isMobile,
      hasTouch: isMobile,
      deviceScaleFactor: isMobile ? 3 : 1,
    })

    test('segmented conversion fields pick in place and keep their rows aligned', async ({ page }, testInfo) => {
      await page.route('**/__unit-fields', route => route.fulfill({
        contentType: 'text/html',
        body: `<html><head><meta name="viewport" content="width=device-width, initial-scale=1"></head><body><div id="app"></div><script type="module">
          import { createApp, h, reactive } from '/node_modules/.vite/deps/vue.js';
          import UnitFields from '/src/features/worksets/UnitFields.vue';
          import '/src/style.css';
          const values = reactive({ mode: 'strict', matched: { lossless: { codec: 'flac' }, encoded: { codec: 'mp3', quality: { kind: 'bitrate', bitrate: 320 } } } });
          createApp({ setup: () => () => h('div', { style: 'width:340px;padding:16px' },
            ['mode', 'matched'].map(unit => h(UnitFields, {
              unit, label: unit, scope: 'test', value: values[unit],
              onChange: value => values[unit] = value
            }))) }).mount('#app');
        </script></body></html>`,
      }))
      await page.setViewportSize(isMobile
        ? { width: 393, height: 852 }
        : { width: 1440, height: 900 })
      await page.goto('/__unit-fields')

      const pick = async (testId: string, name: string) => {
        const button = page.getByTestId(testId).getByRole('button', { name, exact: true })
        if (isMobile)
          await button.tap()
        else await button.click()
        await expect(button).toHaveAttribute('aria-pressed', 'true')
      }

      await pick('test-mode', '可用源')
      await pick('test-matched-encoded', '不需要')
      await expect(page.getByTestId('test-matched-bitrate')).toHaveCount(0)
      await pick('test-matched-encoded', 'Opus')
      await expect(page.getByTestId('test-matched-bitrate')).toBeVisible()
      await pick('test-matched-lossless', '不需要')
      await expect(page.getByTestId('test-matched-encoded').getByRole('button', { name: 'Opus' })).toHaveAttribute('aria-pressed', 'true')

      // Each row label shares its control's centre line.
      for (const [label, testId] of [
        ['无损', 'test-matched-lossless'],
        ['编码', 'test-matched-encoded'],
        ['码率', 'test-matched-bitrate'],
      ]) {
        const row = await page.getByText(label, { exact: true }).boundingBox()
        const control = await page.getByTestId(testId).boundingBox()
        expect(Math.abs(row!.y + row!.height / 2 - (control!.y + control!.height / 2))).toBeLessThan(2)
      }
      await page.screenshot({
        path: testInfo.outputPath(isMobile ? 'mobile-fields.png' : 'desktop-fields.png'),
      })
    })
  })
}
