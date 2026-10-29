<script setup lang="ts">
import { useQuery } from '@tanstack/vue-query'
import { useRoute } from 'vue-router'
import AppShell from '@/components/layout/AppShell.vue'
import TaskToastHost from '@/components/layout/TaskToastHost.vue'
import ToastHost from '@/components/layout/ToastHost.vue'
import { useListeningSync } from '@/composables/use-listening-sync'
import { provideLyricsWindow } from '@/composables/use-lyrics-window'
import { useMediaFactsSync } from '@/composables/use-media-facts-sync'
import { usePlayerKeys } from '@/composables/use-player-keys'
import { useApiClient } from '@/lib/api/client'
import { healthQueryOptions } from '@/queries/health'

const route = useRoute()
useQuery(healthQueryOptions(useApiClient()))
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
