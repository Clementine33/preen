import { mount } from '@vue/test-utils'
import { getActivePinia } from 'pinia'
import { defineComponent } from 'vue'
import { usePlayerKeys } from '@/composables/use-player-keys'

/** The app-level player keys, which a page test renders without App.vue; auto-unmounted like any wrapper. */
export function mountPlayerKeys() {
  return mount(defineComponent({ setup: () => (usePlayerKeys(), () => null) }), { global: { plugins: [getActivePinia()!] } })
}
