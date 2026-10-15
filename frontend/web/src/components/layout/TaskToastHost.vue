<script setup lang="ts">
import { ToastAction, ToastClose, ToastDescription, ToastProvider, ToastRoot, ToastViewport } from 'reka-ui'
import { useTaskToast } from '@/composables/use-task-toast'
import { Button } from '@/components/ui/button'

const { current, dismiss } = useTaskToast()
</script>

<template>
  <ToastProvider :duration="4000" swipe-direction="right">
    <ToastRoot v-for="notice in current ? [current] : []" :key="notice.id" class="app-toast task-toast" data-testid="task-toast"
      @update:open="(open: boolean) => { if (!open) dismiss(notice.id) }">
      <ToastDescription class="text">{{ notice.text }}</ToastDescription>
      <ToastAction v-if="notice.action" as-child :alt-text="notice.action.label">
        <Button variant="outline" size="xs" class="task-toast-action rounded-none" type="button" data-testid="task-toast-action" @click="notice.action.run(); dismiss(notice.id)">{{ notice.action.label }}</Button>
      </ToastAction>
      <ToastClose class="task-toast-close" aria-label="关闭通知"><span aria-hidden="true">×</span><span class="sr-only">关闭通知</span></ToastClose>
    </ToastRoot>
    <ToastViewport class="task-toast-viewport" label="任务通知 ({hotkey})" />
  </ToastProvider>
</template>

<style>
.task-toast-viewport { position: fixed; top: calc(env(safe-area-inset-top, 0px) + var(--sp-4)); right: var(--sp-4); left: var(--sp-4); z-index: 60; display: flex; outline: none; list-style: none; margin: 0; padding: 0; pointer-events: none; }
.task-toast { width: 100%; min-width: 0; flex-wrap: wrap; pointer-events: auto; }
.task-toast .text { flex: 1 1 12rem; min-width: 0; overflow-wrap: anywhere; }
.task-toast .task-toast-action { color: var(--invert-fg); border-color: currentColor; background: transparent; }
.task-toast .task-toast-action:hover { color: var(--invert-bg); background: var(--invert-fg); border-color: var(--invert-fg); }
.task-toast .task-toast-action:focus-visible { outline: 2px solid var(--invert-fg); outline-offset: 2px; box-shadow: none; }
.task-toast-close { flex-shrink: 0; cursor: pointer; padding: var(--sp-1); font-size: var(--fs-m); }
@media (width >= 641px) {
  .task-toast-viewport { left: auto; width: 440px; max-width: calc(100vw - 2 * var(--sp-4)); }
}
</style>
