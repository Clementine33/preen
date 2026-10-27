<script setup lang="ts">
import { ref, watch } from 'vue'
import { Button } from '@/components/ui/button'
import { useToast } from '@/composables/use-toast'
import { errorDetails } from '@/lib/api/error'
import { useOutboundProxy, useSetOutboundProxy } from '@/queries/outbound-proxy'

/**
 * The Outbound Proxy editor: one URL the server uses for metadata sources and
 * cover downloads; empty is direct. It saves on demand, and a refusal stays
 * beside the field with what was typed.
 */
const query = useOutboundProxy()
const save = useSetOutboundProxy()
const toast = useToast()

const url = ref('')
const error = ref('')
// The stored value fills the field when it arrives and after each save.
watch(() => query.data.value?.url, (stored) => { url.value = stored ?? '' }, { immediate: true })

async function submit(): Promise<void> {
  error.value = ''
  try {
    await save.mutateAsync(url.value)
    toast.show({ text: '代理已保存' })
  }
  catch (failure) {
    error.value = errorDetails(failure).message
  }
}
</script>

<template>
  <form class="flex flex-wrap items-start gap-2" data-testid="outbound-proxy" @submit.prevent="submit">
    <input
      v-model="url"
      class="field-input h-11! min-w-0 flex-1 text-[16px]! rail:h-[30px]! rail:text-[length:var(--fs-s)]!"
      placeholder="socks5://127.0.0.1:7890"
      aria-label="代理服务器地址"
      :aria-invalid="error ? true : undefined"
      autocomplete="off"
      spellcheck="false"
      data-testid="outbound-proxy-url"
      @input="error = ''"
    >
    <Button type="submit" class="h-11 rail:h-[30px]" size="sm" variant="outline" :disabled="save.isPending.value" data-testid="outbound-proxy-save">
      保存
    </Button>
    <p v-if="error" class="w-full text-[11px] text-destructive" role="alert" data-testid="outbound-proxy-error">{{ error }}</p>
  </form>
</template>
