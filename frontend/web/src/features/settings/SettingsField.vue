<script setup lang="ts">
import { Button } from '@/components/ui/button'

/**
 * One settings row: label and hint in one column, the control in the other;
 * a narrow container (not the viewport) folds them into one column. ■ 已修改
 * beside the label marks an unsaved edit only, with a 撤销 that puts the saved
 * value back. Restoring defaults is one page-level action, not a per-row one.
 */
defineProps<{
  label: string
  hint?: string
  /** An unsaved edit exists for this field. */
  modified?: boolean
  disabled?: boolean
  /** Prefix of the testids: `<id>-modified`, `<id>-undo`. */
  id: string
}>()
defineEmits<{ undo: [] }>()
</script>

<template>
  <div class="@container border-t border-[var(--wire-fine)] py-2.5 first:border-t-0" :data-testid="`field-${id}`">
    <div class="grid gap-x-4 gap-y-1.5 @lg:grid-cols-[11rem_minmax(0,1fr)]">
      <div class="min-w-0">
        <div class="flex min-h-5 flex-wrap items-center gap-x-2 gap-y-1">
          <span class="text-xs font-medium">{{ label }}</span>
          <template v-if="modified">
            <span class="inline-flex items-center gap-1.5 text-[11px]" :data-testid="`${id}-modified`">
              <span class="size-1.5 bg-[var(--warning-ink)]" aria-hidden="true" />已修改
            </span>
            <Button size="xs" variant="outline" :disabled="disabled" :data-testid="`${id}-undo`" @click="$emit('undo')">
              撤销
            </Button>
          </template>
        </div>
        <p v-if="hint" class="text-[11px] leading-snug text-[var(--text-secondary)]">{{ hint }}</p>
      </div>
      <div class="min-w-0 space-y-1.5">
        <slot />
      </div>
    </div>
  </div>
</template>
