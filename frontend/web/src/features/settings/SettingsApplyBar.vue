<script setup lang="ts">
import { Button } from '@/components/ui/button'

/**
 * The apply bar of common settings. The parent shows it only while there are
 * unsaved changes; the settings head already says which records the save
 * reaches, so the button just says 应用. A refused save explains itself here,
 * beside the button, and the edits stay.
 */
defineProps<{
  applying: boolean
  error: string | null
}>()
defineEmits<{ apply: [], discard: [] }>()
</script>

<template>
  <div class="edge-top bg-background px-3 pt-3 pb-2" role="group" aria-label="应用设置" data-testid="settings-apply-bar">
    <p v-if="error" role="alert" class="mb-1.5 text-[11px] text-[var(--danger-ink)]" data-testid="settings-apply-error">
      {{ error }}
    </p>
    <div class="flex flex-wrap items-center justify-end gap-2">
      <span class="mr-auto text-[11px] text-[var(--text-secondary)]">有未应用的修改</span>
      <Button size="sm" variant="outline" :disabled="applying" data-testid="settings-discard" @click="$emit('discard')">
        放弃修改
      </Button>
      <Button size="sm" :disabled="applying" data-testid="settings-apply" @click="$emit('apply')">
        {{ applying ? '应用中…' : '应用' }}
      </Button>
    </div>
  </div>
</template>
