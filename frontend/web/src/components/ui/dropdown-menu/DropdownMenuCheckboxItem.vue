<script setup lang="ts">
import type { DropdownMenuCheckboxItemEmits, DropdownMenuCheckboxItemProps } from 'reka-ui'

import type { HTMLAttributes } from 'vue'
import { reactiveOmit } from '@vueuse/core'
import {
  DropdownMenuCheckboxItem,
  DropdownMenuItemIndicator,
  useForwardPropsEmits,
} from 'reka-ui'
import { cn } from '@/lib/utils'

const props = defineProps<DropdownMenuCheckboxItemProps & { class?: HTMLAttributes['class'] }>()
const emits = defineEmits<DropdownMenuCheckboxItemEmits>()

const delegatedProps = reactiveOmit(props, 'class')

const forwarded = useForwardPropsEmits(delegatedProps, emits)
</script>

<template>
  <DropdownMenuCheckboxItem
    data-slot="dropdown-menu-checkbox-item"
    v-bind="forwarded"
    :class="cn(
      'relative flex min-h-11 rail:min-h-8 cursor-default items-center gap-2 rounded-none border-b border-[var(--wire-fine)] last:border-b-0 pl-1 pr-2 text-sm outline-hidden select-none data-highlighted:not-data-[state=checked]:bg-accent data-highlighted:shadow-[inset_0_0_0_1px_var(--wire-strong)] data-[state=checked]:bg-primary data-[state=checked]:text-primary-foreground data-[state=checked]:data-highlighted:shadow-[inset_0_0_0_2px_var(--invert-bg),inset_0_0_0_3px_var(--invert-fg)] data-disabled:pointer-events-none data-disabled:opacity-50 [&_svg:not([class*=size-])]:size-4 [&_svg]:pointer-events-none [&_svg]:shrink-0',
      props.class,
    )"
  >
    <span
      class="flex size-3.5 shrink-0 items-center justify-center pointer-events-none"
      data-slot="dropdown-menu-checkbox-item-indicator"
    >
      <DropdownMenuItemIndicator>
        <slot name="indicator-icon">
          <span class="block size-1.5 bg-current" aria-hidden="true" />
        </slot>
      </DropdownMenuItemIndicator>
    </span>
    <slot />
  </DropdownMenuCheckboxItem>
</template>
