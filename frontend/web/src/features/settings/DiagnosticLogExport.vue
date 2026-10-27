<script setup lang="ts">
import { ref } from 'vue'
import { Button } from '@/components/ui/button'
import { useToast } from '@/composables/use-toast'
import { useApiClient } from '@/lib/api/client'

/** Log Export: saves the server's recent Diagnostic Log for a bug report. */
const api = useApiClient()
const toast = useToast()
const pending = ref(false)

async function download(): Promise<void> {
  pending.value = true
  try {
    const { blob, filename } = await api.downloadDiagnosticLog()
    const link = document.createElement('a')
    link.href = URL.createObjectURL(blob)
    link.download = filename
    link.click()
    URL.revokeObjectURL(link.href)
  }
  catch (error) {
    toast.error(error, '日志导出失败')
  }
  finally {
    pending.value = false
  }
}
</script>

<template>
  <Button size="sm" variant="outline" :disabled="pending" data-testid="diagnostic-log-export" @click="download">导出</Button>
</template>
