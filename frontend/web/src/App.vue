<script setup lang="ts">
import { useRoute } from 'vue-router'
import AppShell from '@/components/layout/AppShell.vue'
import TaskToastHost from '@/components/layout/TaskToastHost.vue'
import ToastHost from '@/components/layout/ToastHost.vue'
import { useListeningSync } from '@/composables/use-listening-sync'
import { useMediaFactsSync } from '@/composables/use-media-facts-sync'
import { provideLyricsWindow } from '@/composables/use-lyrics-window'
import { usePlayerKeys } from '@/composables/use-player-keys'
const route = useRoute()
useMediaFactsSync()
useListeningSync()
usePlayerKeys(provideLyricsWindow())
</script>

<template>
  <RouterView v-if="route.name === 'login'" />
  <AppShell v-else>
    <RouterView />
  </AppShell>
  <ToastHost />
  <TaskToastHost />
</template>
