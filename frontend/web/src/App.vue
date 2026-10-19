<script setup lang="ts">
import { useRoute } from 'vue-router'
import AppShell from '@/components/layout/AppShell.vue'
import TaskToastHost from '@/components/layout/TaskToastHost.vue'
import ToastHost from '@/components/layout/ToastHost.vue'
import { useMediaFactsSync } from '@/composables/use-media-facts-sync'
const route = useRoute()
useMediaFactsSync()
</script>

<template>
  <RouterView v-if="route.name === 'login'" />
  <AppShell v-else>
    <RouterView />
  </AppShell>
  <ToastHost />
  <TaskToastHost />
</template>
