import { isCancelledError, VueQueryPlugin } from '@tanstack/vue-query'
import { createPinia } from 'pinia'
import { createApp } from 'vue'
import App from './App.vue'
import { router } from './app/router'
import { redirectToLogin } from './app/auth-navigation'
import { ApiClient, ApiError, apiClientKey } from './lib/api/client'
import { useToast } from './composables/use-toast'
import { desktopAdapterKey } from './lib/desktop/desktop-adapter'
import { createDesktopAdapter } from './lib/desktop/web-adapter'
import { createAppQueryClient } from './queries/query-client'
import './style.css'

// Last line of defence for an error no caller handled: an API refusal keeps its
// message, anything else gets a generic one. Cancellation and an expired
// session (which navigates to login) are not failures to report.
function reportUnhandled(error: unknown): void {
  if (isCancelledError(error) || (error instanceof DOMException && error.name === 'AbortError')) return
  if (error instanceof ApiError && error.status === 401) return
  console.error(error)
  useToast().show({ text: error instanceof ApiError ? error.message : '发生意外错误，请刷新页面重试' })
}

async function bootstrap() {
  window.addEventListener('unhandledrejection', (event) => { reportUnhandled(event.reason) })
  window.addEventListener('pageshow', (event) => {
    if (event.persisted) location.reload()
  })
  const desktopAdapter = createDesktopAdapter()
  const apiClient = new ApiClient({
    ...await desktopAdapter.resolveApi(),
    onUnauthorized: desktopAdapter.platform === 'browser' ? redirectToLogin : undefined,
  })
  const app = createApp(App)
  app.config.errorHandler = reportUnhandled

  app.provide(desktopAdapterKey, desktopAdapter)
  app.provide(apiClientKey, apiClient)
  app
    .use(createPinia())
    .use(router)
    .use(VueQueryPlugin, { queryClient: createAppQueryClient() })
    .mount('#app')
}

void bootstrap()
