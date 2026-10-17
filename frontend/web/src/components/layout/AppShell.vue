<script setup lang="ts">
import { AudioLines } from '@lucide/vue'
import { computed } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { GLOBAL_NAV, SETTINGS_NAV, globalNavOwner, type GlobalNavId } from '@/components/layout/global-nav'
import { useTheme } from '@/composables/use-theme'
import { useLibraryList } from '@/queries/libraries'

/**
 * The application shell owns the whole application height and the global
 * navigation space: a 64px icon rail above 640px application width; at or
 * below it the shell is the page alone, and each page header carries the
 * GlobalMenu, so a phone's bottom edge belongs to the page. The
 * switch is pure CSS (the `rail` variant in style.css); no page re-measures the
 * window.
 */
const route = useRoute()
// Ownership comes from the router's own route name, so a workbench
// child route still lights up 工作集 without a second active-id state.
const current = computed(() => globalNavOwner(route.name))

// The shell is mounted for the whole application lifetime, so it stays the
// long-lived observer of the library list: pages share the same cache entry
// and a scan's terminal refresh always has a live observer, whichever module
// is on screen. The list itself renders on the workbench entry page, not here.
useLibraryList()
// The theme is chosen on 设置; the shell keeps its system-preference listener
// alive while that page is not on screen.
useTheme()

function isCurrent(id: GlobalNavId): boolean {
  return current.value === id
}
</script>

<template>
  <div
    class="rail:grid-cols-[64px_minmax(0,1fr)] rail:grid-rows-[minmax(0,1fr)] grid h-dvh grid-cols-1 grid-rows-[minmax(0,1fr)] overflow-hidden substrate text-foreground"
  >
    <!-- Desktop rail: brand on top, the shared definition in the middle,
         设置 at the bottom. -->
    <aside
      class="rail:flex hidden min-h-0 flex-col border-r border-sidebar-border substrate-panel"
      data-testid="global-rail"
    >
      <div class="grid h-14 shrink-0 place-items-center" data-testid="app-brand">
        <AudioLines class="size-5 text-foreground" aria-hidden="true" />
        <span class="sr-only">Onsei Organizer</span>
      </div>
      <nav aria-label="全局导航" class="flex flex-col gap-1 p-1">
        <RouterLink
          v-for="item in GLOBAL_NAV"
          :key="item.id"
          :to="item.to"
          :aria-current="isCurrent(item.id) ? 'page' : undefined"
          class="flex flex-col items-center gap-1 px-1 py-2 text-[10px] font-medium focus-visible:ring-2 focus-visible:ring-sidebar-ring focus-visible:outline-none"
          :class="isCurrent(item.id) ? 'bg-primary text-primary-foreground' : 'text-sidebar-foreground hover:bg-sidebar-accent'"
        >
          <component :is="item.icon" class="size-4" aria-hidden="true" />
          {{ item.label }}
        </RouterLink>
      </nav>
      <div class="mt-auto flex flex-col gap-1 p-1">
        <RouterLink
          :to="SETTINGS_NAV.to"
          :aria-current="isCurrent(SETTINGS_NAV.id) ? 'page' : undefined"
          class="flex flex-col items-center gap-1 px-1 py-2 text-[10px] font-medium focus-visible:ring-2 focus-visible:ring-sidebar-ring focus-visible:outline-none"
          :class="isCurrent(SETTINGS_NAV.id) ? 'bg-primary text-primary-foreground' : 'text-sidebar-foreground hover:bg-sidebar-accent'"
          data-testid="global-settings"
        >
          <component :is="SETTINGS_NAV.icon" class="size-4" aria-hidden="true" />
          {{ SETTINGS_NAV.label }}
        </RouterLink>
      </div>
    </aside>

    <!-- With no bottom bar, the page is the outermost edge: it owns the safe
         area once. -->
    <main class="min-h-0 min-w-0 overflow-y-auto pb-[env(safe-area-inset-bottom)]">
      <slot />
    </main>
  </div>
</template>
