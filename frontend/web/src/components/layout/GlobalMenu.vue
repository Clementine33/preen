<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { GLOBAL_NAV, globalNavOwner, SETTINGS_NAV } from '@/components/layout/global-nav'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

/**
 * The phone's global navigation: the brand mark at the head of
 * every page header opens the global entries. It exists
 * only below the rail breakpoint, so the bottom edge of a phone belongs to the
 * page — the player and the file bay, or a selection's actions.
 */

const route = useRoute()
const router = useRouter()
const ENTRIES = [...GLOBAL_NAV, SETTINGS_NAV]
const current = computed(() => globalNavOwner(route.name))
const currentLabel = computed(() => ENTRIES.find(item => item.id === current.value)?.label ?? '')

function go(value: unknown): void {
  const item = ENTRIES.find(entry => entry.id === value)
  if (item)
    void router.push(item.to)
}
</script>

<template>
  <DropdownMenu>
    <DropdownMenuTrigger as-child>
      <button
        type="button"
        :aria-label="currentLabel ? `全局导航（当前：${currentLabel}）` : '全局导航'"
        class="rail:hidden grid size-11 shrink-0 place-items-center text-foreground hover:bg-accent focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none aria-expanded:bg-primary aria-expanded:text-primary-foreground"
        data-testid="global-menu"
      >
        <img src="/favicon.svg" class="size-8" alt="" aria-hidden="true">
      </button>
    </DropdownMenuTrigger>
    <DropdownMenuContent side="bottom" align="start" :side-offset="4" class="w-48 p-0">
      <div class="strip m-2 mb-1">
        <span class="strip-name lbl">NAV</span>
      </div>
      <DropdownMenuRadioGroup :model-value="current ?? undefined" class="px-2 pb-2" @update:model-value="go">
        <DropdownMenuRadioItem
          v-for="item in ENTRIES"
          :key="item.id"
          :value="item.id"
          class="cursor-pointer text-[length:var(--fs-s)] font-medium"
        >
          {{ item.label }}
        </DropdownMenuRadioItem>
      </DropdownMenuRadioGroup>
    </DropdownMenuContent>
  </DropdownMenu>
</template>
