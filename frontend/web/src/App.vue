<script setup lang="ts">
import AppShell from '@/components/layout/AppShell.vue'
import { useMediaFactsSync } from '@/composables/use-media-facts-sync'
useMediaFactsSync()
</script>

<template>
  <AppShell>
    <RouterView />
  </AppShell>
</template>
