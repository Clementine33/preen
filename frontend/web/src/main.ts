import { VueQueryPlugin } from '@tanstack/vue-query'
import { createPinia } from 'pinia'
import { createApp } from 'vue'
import App from './App.vue'
import { router } from './app/router'
import { redirectToLogin } from './app/auth-navigation'
import { ApiClient, apiClientKey } from './lib/api/client'
import { desktopAdapterKey } from './lib/desktop/desktop-adapter'
import { createDesktopAdapter } from './lib/desktop/web-adapter'
import { createAppQueryClient } from './queries/query-client'
import './style.css'

async function bootstrap() {
  window.addEventListener('pageshow', (event) => {
    if (event.persisted) location.reload()
  })
  const desktopAdapter = createDesktopAdapter()
  const apiClient = new ApiClient({
    ...await desktopAdapter.resolveApi(),
    onUnauthorized: desktopAdapter.platform === 'browser' ? redirectToLogin : undefined,
  })
  const app = createApp(App)

  app.provide(desktopAdapterKey, desktopAdapter)
  app.provide(apiClientKey, apiClient)
  app
    .use(createPinia())
    .use(router)
    .use(VueQueryPlugin, { queryClient: createAppQueryClient() })
    .mount('#app')
}

void bootstrap()
