<script setup lang="ts">
import type { OutboundProxy } from '@/lib/api/types'
import { ref, watch } from 'vue'
import { Button } from '@/components/ui/button'
import { useToast } from '@/composables/use-toast'
import SettingsSegmented from '@/features/settings/SettingsSegmented.vue'
import { errorDetails } from '@/lib/api/error'
import { useOutboundProxy, useSetOutboundProxy } from '@/queries/outbound-proxy'

/**
 * The Outbound Proxy editor: how the server reaches metadata sources and
 * cover downloads — following the system's proxy, through one typed URL, or
 * directly. Following the system or going direct saves on the click; a typed
 * URL saves on demand, and a refusal stays beside the field with what was typed.
 */
const MODES = [
  { value: 'system', label: '跟随系统' },
  { value: 'manual', label: '手动' },
  { value: 'direct', label: '直连' },
]

const query = useOutboundProxy()
const save = useSetOutboundProxy()
const toast = useToast()

const mode = ref<OutboundProxy['mode']>('system')
const url = ref('')
const error = ref('')
// The stored value fills the form when it arrives and after each save.
watch(() => query.data.value, (stored) => {
  mode.value = stored?.mode ?? 'system'
  url.value = stored?.url ?? ''
}, { immediate: true })

async function store(input: OutboundProxy): Promise<void> {
  error.value = ''
  try {
    await save.mutateAsync(input)
    toast.show({ text: '代理已保存' })
  }
  catch (failure) {
    error.value = errorDetails(failure).message
    if (input.mode !== 'manual')
      mode.value = query.data.value?.mode ?? 'system'
  }
}

function pick(next: string): void {
  mode.value = next as OutboundProxy['mode']
  error.value = ''
  // The stored URL is kept, not the field's unsaved text.
  if (next !== 'manual')
    void store({ mode: mode.value, url: query.data.value?.url ?? '' })
}
</script>

<template>
  <div class="space-y-2" data-testid="outbound-proxy-settings">
    <SettingsSegmented :options="MODES" :value="mode" label="代理方式" :disabled="save.isPending.value" @change="pick" />
    <form v-if="mode === 'manual'" class="flex flex-wrap items-start gap-2" data-testid="outbound-proxy" @submit.prevent="store({ mode: 'manual', url })">
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
    </form>
    <p v-if="error" class="w-full text-[11px] text-destructive" role="alert" data-testid="outbound-proxy-error">{{ error }}</p>
  </div>
</template>
