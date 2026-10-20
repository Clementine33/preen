<script setup lang="ts">
import { ToastAction, ToastClose, ToastDescription, ToastProvider, ToastRoot, ToastTitle, ToastViewport } from 'reka-ui'
import { useTaskToast } from '@/composables/use-task-toast'
import type { TaskTone } from '@/stores/task-toast'
import { Button } from '@/components/ui/button'

const { current, dismiss } = useTaskToast()

// Severity is spelled out in the tag, never carried by color alone.
const TAGS: Record<TaskTone, string> = { success: '完成', stopped: '已停止', partial: '部分失败', failed: '失败', interrupted: '中断', unknown: '待确认' }
const STICKY = new Set<TaskTone>(['partial', 'failed', 'interrupted', 'unknown'])
const duration = (tone: TaskTone) => STICKY.has(tone) ? Infinity : 8000
</script>

<template>
  <ToastProvider swipe-direction="right">
    <ToastRoot v-for="notice in current ? [current] : []" :key="notice.id" :duration="duration(notice.tone)"
      class="task-toast" :data-tone="notice.tone" :data-sticky="STICKY.has(notice.tone) || undefined" data-testid="task-toast"
      @update:open="(open: boolean) => { if (!open) dismiss(notice.id) }">
      <div class="task-toast-head">
        <span class="task-toast-tag">{{ TAGS[notice.tone] }}</span>
        <ToastTitle class="task-toast-title">{{ notice.title }}</ToastTitle>
        <ToastClose class="task-toast-close" aria-label="关闭通知"><span aria-hidden="true">×</span></ToastClose>
      </div>
      <ToastDescription class="task-toast-text">{{ notice.text }}</ToastDescription>
      <ToastAction v-if="notice.action" as-child :alt-text="notice.action.label">
        <Button variant="outline" size="xs" class="task-toast-action rounded-none" type="button" data-testid="task-toast-action" @click="notice.action.run(); dismiss(notice.id)">{{ notice.action.label }}</Button>
      </ToastAction>
    </ToastRoot>
    <ToastViewport class="task-toast-viewport" label="任务通知 ({hotkey})" />
  </ToastProvider>
</template>

<style>
.task-toast-viewport { position: fixed; top: calc(env(safe-area-inset-top, 0px) + var(--sp-4)); right: var(--sp-4); left: var(--sp-4); z-index: 60; display: flex; outline: none; list-style: none; margin: 0; padding: 0; pointer-events: none; }
.task-toast {
  width: 100%; min-width: 0; pointer-events: auto;
  display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: var(--sp-2) var(--sp-3); align-items: end;
  padding: var(--sp-3); background: var(--bg-panel); color: var(--fg-base);
  border: 1px solid var(--wire-strong); box-shadow: 4px 4px 0 var(--wire-fine);
}
.task-toast[data-sticky] { border-left-width: 4px; }
.task-toast-head { grid-column: 1 / -1; display: flex; align-items: center; gap: var(--sp-2); min-width: 0; }
.task-toast-tag {
  flex-shrink: 0; padding: 1px var(--sp-1); border: 1px solid var(--wire-strong);
  font-family: var(--font-mono); font-size: var(--fs-xs); letter-spacing: .08em; line-height: 1.4;
}
.task-toast[data-tone="failed"] .task-toast-tag, .task-toast[data-tone="partial"] .task-toast-tag { background: var(--invert-bg); color: var(--invert-fg); }
.task-toast[data-tone="unknown"] .task-toast-tag, .task-toast[data-tone="interrupted"] .task-toast-tag { border-style: dashed; }
.task-toast-title {
  flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  font-family: var(--font-mono); font-size: var(--fs-xs); letter-spacing: .06em; color: var(--fg-meta);
}
.task-toast-close { flex-shrink: 0; cursor: pointer; padding: 0 var(--sp-1); font-size: var(--fs-m); line-height: 1; color: var(--fg-muted); }
.task-toast-close:hover { color: var(--fg-base); }
.task-toast-close:focus-visible { outline: 2px solid var(--wire-strong); outline-offset: 2px; }
.task-toast-text { min-width: 0; font-size: var(--fs-m); line-height: 1.5; overflow-wrap: anywhere; }
.task-toast-action { flex-shrink: 0; }
@media (width >= 641px) {
  .task-toast-viewport { left: auto; width: 440px; max-width: calc(100vw - 2 * var(--sp-4)); }
}
.task-toast { transition: opacity 160ms var(--ease-out), transform 160ms var(--ease-out); }
@starting-style {
  .task-toast[data-state="open"] { opacity: 0; transform: translateY(-6px); }
}
</style>
