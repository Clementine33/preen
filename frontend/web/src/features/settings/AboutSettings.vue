<script setup lang="ts">
import { useQuery } from '@tanstack/vue-query'
import { Button } from '@/components/ui/button'
import { useApiClient } from '@/lib/api/client'
import { healthQueryOptions } from '@/queries/health'
import SettingsField from './SettingsField.vue'

const REPO_URL = 'https://github.com/Clementine33/preen'

/** 关于: the product name, the server's build version (`GET /health`), the repository link and the licence. */
const api = useApiClient()
const health = useQuery(healthQueryOptions(api))
</script>

<template>
  <div data-testid="settings-about">
    <SettingsField id="about-name" label="应用">Preen</SettingsField>
    <SettingsField id="about-version" label="版本">
      <span data-testid="about-version">{{ health.data.value?.version ?? '—' }}</span>
      <Button v-if="health.isError.value" size="xs" variant="outline" class="ml-2" @click="health.refetch()">重试</Button>
    </SettingsField>
    <SettingsField id="about-repo" label="项目仓库">
      <Button as="a" size="sm" variant="outline" :href="REPO_URL" target="_blank" rel="noopener noreferrer" data-testid="about-repo">GitHub</Button>
    </SettingsField>
    <SettingsField id="about-license" label="许可证">GPL-3.0</SettingsField>
  </div>
</template>
