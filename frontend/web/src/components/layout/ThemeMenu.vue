<script setup lang="ts">
import { Monitor, Moon, Sun } from '@lucide/vue'
import { computed } from 'vue'
import { DropdownMenu, DropdownMenuContent, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import ThemeRadioGroup from '@/components/layout/ThemeRadioGroup.vue'
import { THEME_OPTIONS, useTheme, type Theme } from '@/composables/use-theme'

/**
 * The desktop theme entry, at the foot of the rail. Phones reach
 * the same group through the GlobalMenu in each page header. The trigger reads
 * like a nav entry and names the mode in force; the menu is a radio group, so
 * the chosen mode is announced and marked, not just applied.
 */

const { theme } = useTheme()
const ICONS: Record<Theme, typeof Sun> = { light: Sun, dark: Moon, system: Monitor }
const current = computed(() => THEME_OPTIONS.find((option) => option.value === theme.value) ?? THEME_OPTIONS[1])
</script>

<template>
  <DropdownMenu>
    <DropdownMenuTrigger as-child>
      <button
        type="button"
        :aria-label="`切换主题（当前：${current.label}）`"
        class="flex w-full flex-col items-center justify-center gap-1 py-2 text-sidebar-foreground hover:bg-sidebar-accent focus-visible:ring-2 focus-visible:ring-sidebar-ring focus-visible:outline-none aria-expanded:bg-primary aria-expanded:text-primary-foreground"
        data-testid="theme-menu"
      >
        <component :is="ICONS[current.value]" class="size-4" aria-hidden="true" />
        <span class="font-mono text-[10px] leading-3 tracking-[.14em]">{{ current.code }}</span>
      </button>
    </DropdownMenuTrigger>
    <DropdownMenuContent side="right" align="end" :side-offset="8" class="w-40 p-0">
      <ThemeRadioGroup />
    </DropdownMenuContent>
  </DropdownMenu>
</template>
