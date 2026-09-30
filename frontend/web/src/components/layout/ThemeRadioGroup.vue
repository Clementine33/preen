<script setup lang="ts">
import { DropdownMenuRadioGroup, DropdownMenuRadioItem } from '@/components/ui/dropdown-menu'
import { THEME_OPTIONS, useTheme } from '@/composables/use-theme'

/**
 * The THEME section of a menu: the rail's ThemeMenu and the phone GlobalMenu
 * both render this one group, so the two theme entries cannot drift apart.
 */

const { theme } = useTheme()

function choose(value: unknown): void {
  if (value === 'light' || value === 'dark' || value === 'system') theme.value = value
}
</script>

<template>
  <!-- The strip's ■ and the checked row's ■ share one axis: 8 + 8 + 3 = 8 + 4 + 7. -->
  <div class="strip m-2 mb-1">
    <span class="strip-name lbl">THEME</span>
  </div>
  <DropdownMenuRadioGroup :model-value="theme" class="px-2 pb-2" @update:model-value="choose">
    <DropdownMenuRadioItem
      v-for="option in THEME_OPTIONS"
      :key="option.value"
      :value="option.value"
      class="cursor-pointer text-[length:var(--fs-s)] font-medium"
    >
      {{ option.label }}
    </DropdownMenuRadioItem>
  </DropdownMenuRadioGroup>
</template>
