import type { Plugin, UserConfig } from 'vite'
import { defineConfig, mergeConfig } from 'vitest/config'
import viteConfig from './vite.config.ts'

// vite.config.ts uses the command callback form; vitest needs the resolved
// object, so resolve it as 'serve'.
const resolvedViteConfig: UserConfig
  = typeof viteConfig === 'function'
    ? viteConfig({ command: 'serve', mode: 'test' })
    : viteConfig

// plugin-vue turns a template's `src="/x.svg"` into an import of a public-dir
// file; resolved as a file:/// URL that is not an absolute path on Windows. A
// test only needs the address itself, so answer it without touching the disk.
const publicAssetUrl: Plugin = {
  name: 'test-public-asset-url',
  enforce: 'pre',
  resolveId: id => (/^\/[^/]+\.svg$/.test(id)
    ? `\0public:${id}`
    : null),
  load: id => (id.startsWith('\0public:')
    ? `export default ${JSON.stringify(id.slice('\0public:'.length))}`
    : null),
}

export default mergeConfig(
  resolvedViteConfig,
  defineConfig({
    plugins: [publicAssetUrl],
    test: {
      environment: 'jsdom',
      include: ['src/**/*.test.ts', 'src/**/*.spec.ts'],
      setupFiles: ['./src/test/setup.ts'],
      css: false,
    },
  }),
)
