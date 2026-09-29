<script setup lang="ts">
import { Monitor, Moon, Sun } from '@lucide/vue'
import { computed } from 'vue'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { useTheme, type Theme } from '@/composables/use-theme'

/**
 * The application's only theme entry (ADR 0012 §4). The shell renders it at
 * the foot of the desktop rail; phones have no theme control until a settings
 * page exists, and no page carries one of its own. The trigger reads like a nav
 * entry and names the mode in force; the menu is a radio group, so the chosen
 * mode is announced and marked, not just applied.
 */

const { theme } = useTheme()
const OPTIONS: { value: Theme; label: string; code: string; icon: typeof Sun }[] = [
  { value: 'light', label: '浅色', code: 'LIGHT', icon: Sun },
  { value: 'dark', label: '深色', code: 'DARK', icon: Moon },
  { value: 'system', label: '跟随系统', code: 'AUTO', icon: Monitor },
]
const current = computed(() => OPTIONS.find((option) => option.value === theme.value) ?? OPTIONS[1])

function choose(value: unknown): void {
  if (value === 'light' || value === 'dark' || value === 'system') theme.value = value
}
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
        <component :is="current.icon" class="size-4" aria-hidden="true" />
        <span class="font-mono text-[10px] leading-3 tracking-[.14em]">{{ current.code }}</span>
      </button>
    </DropdownMenuTrigger>
    <DropdownMenuContent side="right" align="end" :side-offset="8" class="w-40 p-0">
      <DropdownMenuLabel class="strip">
        <span class="strip-name"><span class="lbl">THEME</span></span>
      </DropdownMenuLabel>
      <DropdownMenuRadioGroup :model-value="theme" class="p-1" @update:model-value="choose">
        <DropdownMenuRadioItem v-for="option in OPTIONS" :key="option.value" :value="option.value">
          <component :is="option.icon" class="size-4" aria-hidden="true" />
          {{ option.label }}
          <template #indicator-icon>
            <span class="block size-1.5 bg-current" aria-hidden="true" />
          </template>
        </DropdownMenuRadioItem>
      </DropdownMenuRadioGroup>
    </DropdownMenuContent>
  </DropdownMenu>
</template>
