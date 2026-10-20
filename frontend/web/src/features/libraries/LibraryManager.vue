<script setup lang="ts">
import { ref, watch } from 'vue'
import { FolderOpen } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import Modal from '@/components/ui/modal/Modal.vue'
import type { CreateLibraryInput, Library, StorageClass } from '@/lib/api/types'
import { useDesktopAdapter } from '@/lib/desktop/desktop-adapter'

const props = defineProps<{
  open: boolean
  library?: Library | null
  saving?: boolean
}>()
const emit = defineEmits<{
  close: []
  save: [value: CreateLibraryInput & { storage_class: StorageClass }]
  remove: [id: string]
}>()

const adapter = useDesktopAdapter()
const name = ref('')
const rootPath = ref('')
const ssd = ref(false)

watch(
  () => [props.open, props.library] as const,
  () => {
    if (!props.open) return
    name.value = props.library?.name ?? ''
    rootPath.value = props.library?.root_path ?? ''
    ssd.value = props.library?.storage_class === 'ssd'
  },
  { immediate: true },
)

async function pickFolder() {
  const picked = await adapter.pickFolder()
  if (picked !== null) rootPath.value = picked
}

function submit() {
  if (!name.value || !rootPath.value) return
  emit('save', { name: name.value, root_path: rootPath.value, storage_class: ssd.value ? 'ssd' : 'hdd' })
}
</script>

<template>
  <Modal
    :open="open"
    :title="library ? '编辑媒体库' : '添加媒体库'"
    description="每个条目指向一个音频根目录。可以添加多个媒体库。"
    class="pb-[max(1.25rem,env(safe-area-inset-bottom))]"
    @close="emit('close')"
  >
    <form @submit.prevent="submit">
      <label class="block text-xs font-medium" for="library-name">名称</label>
      <input
        id="library-name"
        v-model="name"
        required
        autocomplete="off"
        class="mt-1.5 h-11 w-full rounded-md border border-input bg-transparent px-3 text-[16px] focus-visible:border-[var(--wire-strong)] focus-visible:outline-none rail:h-9 rail:text-sm"
        placeholder="例如：无损音乐库"
      />

      <label class="mt-4 block text-xs font-medium" for="library-root">根目录</label>
      <div class="mt-1.5 flex gap-2">
        <input
          id="library-root"
          v-model="rootPath"
          required
          autocomplete="off"
          spellcheck="false"
          class="h-11 min-w-0 flex-1 rounded-md border border-input bg-transparent px-3 font-mono text-[16px] focus-visible:border-[var(--wire-strong)] focus-visible:outline-none rail:h-9 rail:text-xs"
          placeholder="/home/me/music 或 D:\Music"
        />
        <Button class="h-11 shrink-0 rail:h-8" type="button" variant="outline" @click="pickFolder">
          <FolderOpen class="size-4" />
          选择文件夹
        </Button>
      </div>
      <p class="mt-1.5 text-[11px] text-muted-foreground">
        浏览器无法打开系统目录选择器时，请直接粘贴路径。路径会原样发送给后端。
      </p>

      <label class="mt-4 flex min-h-11 cursor-pointer items-start gap-3 rail:min-h-0">
        <input v-model="ssd" type="checkbox" class="mt-0.5 size-4 shrink-0" data-testid="library-ssd" />
        <span>
          <span class="block text-xs font-medium">媒体在 SSD 上</span>
          <span class="mt-0.5 block text-[11px] text-muted-foreground">
            决定读取媒体信息和转换时的并行宽度。
          </span>
        </span>
      </label>

      <div class="mt-6 flex flex-wrap items-center gap-2 border-t border-border pt-4">
        <Button
          v-if="library"
          class="h-11 rail:h-8"
          type="button"
          variant="destructive"
          :disabled="saving"
          @click="emit('remove', library.id)"
        >
          删除媒体库
        </Button>
        <Button type="button" variant="ghost" class="ml-auto h-11 rail:h-8" @click="emit('close')">取消</Button>
        <Button class="h-11 rail:h-8" type="submit" :disabled="saving || !name || !rootPath">
          {{ saving ? '保存中…' : '保存' }}
        </Button>
      </div>
    </form>
  </Modal>
</template>
