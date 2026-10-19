import { fileURLToPath, URL } from 'node:url'
import tailwindcss from '@tailwindcss/vite'
import vue from '@vitejs/plugin-vue'
import { defineConfig } from 'vite'
import VueDevTools from 'vite-plugin-vue-devtools'


// https://vite.dev/config/
export default defineConfig(() => ({
  plugins: [vue(), tailwindcss(), VueDevTools()],
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
