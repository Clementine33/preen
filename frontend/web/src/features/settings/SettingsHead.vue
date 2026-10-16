<script setup lang="ts">
import { Button } from '@/components/ui/button'

/**
 * The head of one Task kind's settings: what the settings reach, and the one
 * 恢复默认 of the page. Restoring fills every field with its default as
 * unsaved edits, so it still goes through the apply bar and can be discarded.
 */
defineProps<{
  scope: string
  /** Some field shows a value other than its default. */
  canRestore: boolean
  disabled?: boolean
}>()
defineEmits<{ restore: [] }>()
</script>

<template>
  <div class="flex items-start gap-3" data-testid="settings-head">
    <p class="flex-1 text-[11px] leading-snug text-[var(--text-secondary)]">{{ scope }}</p>
    <Button
      size="xs"
      variant="outline"
      :disabled="disabled || !canRestore"
      data-testid="settings-restore-defaults"
      @click="$emit('restore')"
    >
      恢复默认
    </Button>
  </div>
</template>
