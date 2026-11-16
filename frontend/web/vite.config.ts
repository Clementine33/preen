import type { Plugin } from 'vite'
import { fileURLToPath, URL } from 'node:url'
import tailwindcss from '@tailwindcss/vite'
import vue from '@vitejs/plugin-vue'
import { defineConfig } from 'vite'
import VueDevTools from 'vite-plugin-vue-devtools'

// @fontsource lists a woff fallback after every woff2. Every supported engine
// reads woff2, and the fallbacks would double the files in dist (and in the
// Desktop Shell's embedded assets).
function woff2Only(): Plugin {
  return {
    name: 'woff2-only',
    enforce: 'pre',
    transform(code, id) {
      if (id.includes('/@fontsource') && id.endsWith('.css'))
        return code.replace(/, url\([^)]+\.woff\) format\('woff'\)/g, '')
    },
  }
}

// A production build must never render the development contract checker,
// schema loader, AJV, or the generated binding metadata.
function excludeWireValidation(): Plugin {
  return {
    name: 'exclude-wire-validation',
    apply: 'build',
    generateBundle(_options, bundle) {
      for (const chunk of Object.values(bundle)) {
        if (chunk.type !== 'chunk')
          continue
        for (const [id, module] of Object.entries(chunk.modules)) {
          if (module.renderedLength > 0 && (id.includes('/wire-validation.ts') || id.includes('/docs/api/schema/') || id.includes('/contracts.gen.ts') || id.includes('/node_modules/ajv/')))
            this.error(`Production contains wire validation: ${id}`)
        }
      }
      this.info('Wire validation excluded: zero rendered checker, validator, binding or AJV modules')
    },
  }
}

// https://vite.dev/config/
export default defineConfig(({ mode }) => ({
  // The Desktop Shell's dev mode drops the in-page Vue devtools, which slow
  // WebKitGTK markedly; the WebKit inspector remains.
  plugins: [
    ...(mode === 'production'
      ? [
          excludeWireValidation(),
        ]
      : []),
    woff2Only(),
    vue(),
    tailwindcss(),
    ...(process.env.PREEN_VUE_DEVTOOLS === '0'
      ? []
      : [
          VueDevTools(),
        ]),
  ],
  server: {
    proxy: process.env.PREEN_BACKEND_URL
      ? {
          '/api': {
            target: process.env.PREEN_BACKEND_URL,
            headers: process.env.PREEN_TOKEN
              ? { Authorization: `Bearer ${process.env.PREEN_TOKEN}` }
              : undefined,
          },
        }
      : undefined,
  },
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
}))
