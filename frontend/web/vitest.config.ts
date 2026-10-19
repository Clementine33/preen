import { defineConfig, mergeConfig } from 'vitest/config'
import type { UserConfig } from 'vite'
import viteConfig from './vite.config.ts'

// vite.config.ts uses the command callback form; vitest needs the resolved
// object, so resolve it as 'serve'.
const resolvedViteConfig: UserConfig =
  typeof viteConfig === 'function' ? viteConfig({ command: 'serve', mode: 'test' }) : viteConfig

export default mergeConfig(
  resolvedViteConfig,
  defineConfig({
    test: {
      environment: 'jsdom',
      include: ['src/**/*.test.ts', 'src/**/*.spec.ts'],
      setupFiles: ['./src/test/setup.ts'],
      css: false,
    },
  }),
)