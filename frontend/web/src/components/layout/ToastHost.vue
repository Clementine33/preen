<script setup lang="ts">
import { ToastAction, ToastDescription, ToastProvider, ToastRoot, ToastViewport } from 'reka-ui'
import { useToast } from '@/composables/use-toast'

// The app-level toast viewport, bottom-centre above the PlayerBar. Reka holds
// the timer while the toast is hovered or focused; a new message is a new key,
// so it replaces the old one outright.
const { current, dismiss } = useToast()
</script>

<template>
  <ToastProvider :duration="4000" swipe-direction="down">
    <ToastRoot v-if="current" :key="current.id" class="app-toast" data-testid="toast"
      @update:open="(open: boolean) => { if (!open) dismiss(current!.id) }">
      <ToastDescription class="text">{{ current.text }}</ToastDescription>
      <ToastAction v-if="current.action" as-child :alt-text="current.action.label">
        <button class="btn btn-s" type="button" data-testid="toast-action" @click="current.action.run(); dismiss(current.id)">{{ current.action.label }}</button>
      </ToastAction>
    </ToastRoot>
    <ToastViewport class="app-toast-viewport" />
  </ToastProvider>
</template>

<!-- Unscoped: reka renders the viewport and the toast outside this component's scope. -->
<style>
.app-toast-viewport { position: fixed; left: 50%; bottom: 44px; transform: translateX(-50%); z-index: 60; display: flex; max-width: calc(100vw - 2 * var(--sp-4)); outline: none; list-style: none; margin: 0; padding: 0; }
.app-toast {
  display: flex; align-items: center; gap: var(--sp-4); padding: var(--sp-2) var(--sp-3);
  background: var(--invert-bg); color: var(--invert-fg); border: 1px solid var(--wire-strong);
  font-family: var(--font-mono); font-size: var(--fs-s); letter-spacing: .04em;
}
.app-toast .btn { color: inherit; border-color: currentColor; flex-shrink: 0; }
</style>
