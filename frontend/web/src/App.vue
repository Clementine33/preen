<script setup lang="ts">
import AppShell from '@/components/layout/AppShell.vue'
import ToastHost from '@/components/layout/ToastHost.vue'
import { useMediaFactsSync } from '@/composables/use-media-facts-sync'
useMediaFactsSync()
</script>

<template>
  <AppShell>
    <RouterView />
  </AppShell>
  <ToastHost />
</template>
