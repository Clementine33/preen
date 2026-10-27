<script setup lang="ts">
import type { CreateLibraryInput, StorageClass } from '@/lib/api/types'
import { AlertTriangle, LibraryBig, Pencil, Plus, RefreshCw } from '@lucide/vue'
import { useMutation, useQueryClient } from '@tanstack/vue-query'
import { computed, ref } from 'vue'
import { RouterLink } from 'vue-router'
import GlobalMenu from '@/components/layout/GlobalMenu.vue'
import { Button } from '@/components/ui/button'
import LibraryManager from '@/features/libraries/LibraryManager.vue'
import { useApiClient } from '@/lib/api/client'
import { errorDetails } from '@/lib/api/error'
import {
  createLibraryMutationOptions,
  deleteLibraryMutationOptions,
  updateLibraryMutationOptions,
  useLibraryList,
} from '@/queries/libraries'
import { useLibraryUiStore } from '@/stores/library-ui'

/**
 * 工作集 — the media-library selection (`/worksets`, ADR 0001; ADR 0003).
 *
 * The product entry keeps the name 工作集, and what it selects is a media
 * library: choosing one enters its workbench, where browsing, conversion and
 * file management all happen. The old global 媒体库 entry is gone with this
 * page, and no old link is maintained.
 */
const api = useApiClient()
const queryClient = useQueryClient()
const ui = useLibraryUiStore()

const { query, librariesData } = useLibraryList()
const libraries = computed(() => librariesData.value)
const loading = computed(() => query.isPending.value && !query.data.value)

const managerOpen = ref(false)
const editing = ref(false)
const saving = ref(false)

const createMutation = useMutation(createLibraryMutationOptions(api, queryClient))
const updateMutation = useMutation(updateLibraryMutationOptions(api, queryClient))
const deleteMutation = useMutation(deleteLibraryMutationOptions(api, queryClient))

const editingLibrary = computed(() =>
  editing.value
    ? (libraries.value.find(library => library.id === ui.activeLibraryId) ?? null)
    : null,
)

const pageError = computed(() => {
  const queryError = query.error.value
  if (queryError)
    return errorDetails(queryError)
  for (const mutation of [createMutation, updateMutation, deleteMutation]) {
    if (mutation.error.value)
      return errorDetails(mutation.error.value)
  }
  return null
})

function openAdd() {
  editing.value = false
  managerOpen.value = true
}

function openEdit(libraryId: string) {
  editing.value = true
  ui.setActiveLibrary(libraryId)
  managerOpen.value = true
}

async function saveLibrary({ storage_class, ...input }: CreateLibraryInput & { storage_class: StorageClass }) {
  saving.value = true
  try {
    if (editing.value && editingLibrary.value) {
      await updateMutation.mutateAsync({
        id: editingLibrary.value.id,
        input: { ...input, storage_class },
      })
    }
    else {
      const created = await createMutation.mutateAsync(input)
      ui.setActiveLibrary(created.id)
      // Create takes no storage class; a new library starts on HDD. Once it
      // exists, a failed follow-up reopens it as an edit rather than a re-create.
      editing.value = true
      if (storage_class !== 'hdd') {
        await updateMutation.mutateAsync({
          id: created.id,
          input: { storage_class },
        })
      }
    }
    managerOpen.value = false
  }
  catch {
    // Mutation errors surface in the page banner; the dialog stays open.
  }
  finally {
    saving.value = false
  }
}

async function removeLibrary(id: string) {
  if (
    !window.confirm(
      '删除这个媒体库条目？它的设置、处理记录、计划和执行结果都会一并删除；磁盘上的音频文件与 Delete/ 回收目录不会被删除。',
    )
  ) {
    return
  }
  saving.value = true
  try {
    await deleteMutation.mutateAsync(id)
    managerOpen.value = false
  }
  catch {
    // Keep the dialog open and expose the mutation error in the banner.
  }
  finally {
    saving.value = false
  }
}

async function retryPage() {
  for (const mutation of [createMutation, updateMutation, deleteMutation]) mutation.reset()
  await query.refetch()
}
</script>

<template>
  <section class="flex h-full min-w-0 flex-col substrate" data-testid="worksets-page">
    <header class="flex min-h-14 shrink-0 flex-wrap items-center gap-x-3 gap-y-1.5 edge-bottom px-4 py-1.5 rail:px-5">
      <GlobalMenu />
      <div class="min-w-0 flex-1 rail:flex-none">
        <h1 class="font-heading text-sm font-semibold">工作集</h1>
        <p class="truncate text-[11px] text-muted-foreground">选择一个媒体库进入工作台</p>
      </div>
      <Button class="ml-auto h-11 rail:h-7" variant="outline" size="sm" @click="openAdd">
        <Plus class="size-3.5" />
        添加媒体库
      </Button>
    </header>

    <div
      v-if="pageError"
      data-testid="page-error"
      class="flex items-center gap-3 border-b border-destructive/30 bg-destructive/10 px-5 py-3 text-xs text-destructive"
    >
      <AlertTriangle class="size-4 shrink-0" />
      <span v-if="pageError.code" class="shrink-0 font-mono font-semibold">{{ pageError.code }}</span>
      <span class="min-w-0 flex-1">{{ pageError.message }}</span>
      <Button data-testid="retry-page" class="h-11 rail:h-7" variant="outline" size="sm" @click="retryPage">
        <RefreshCw class="size-3.5" />
        重试
      </Button>
    </div>

    <div class="min-h-0 flex-1 overflow-y-auto">
      <div v-if="loading" class="grid h-full place-items-center text-xs text-muted-foreground">
        正在连接媒体库…
      </div>

      <div
        v-else-if="libraries.length === 0"
        class="grid h-full place-items-center px-6 text-center"
        data-testid="libraries-empty"
      >
        <div class="max-w-md border border-dashed border-[var(--wire-mid)] px-8 py-10">
          <div class="hatch mx-auto size-10 border border-border">
            <Plus class="size-4 text-muted-foreground" />
          </div>
          <h1 class="mt-4 font-heading text-lg font-semibold">还没有媒体库</h1>
          <p class="mt-2 text-xs leading-5 text-muted-foreground">
            添加一个音频根目录，扫描它的直接子文件夹，然后进入工作台浏览与转换。
          </p>
          <Button data-testid="empty-add-library" class="mt-5 h-11 rail:h-7" size="sm" @click="openAdd">
            添加媒体库
          </Button>
        </div>
      </div>

      <ul v-else class="mx-auto max-w-3xl space-y-1.5 p-4">
        <li v-for="library in libraries" :key="library.id" class="flex items-center gap-2">
          <RouterLink
            :to="{ name: 'workbench-overview', params: { libraryId: library.id } }"
            class="flex min-w-0 flex-1 items-center gap-2 border border-[var(--wire-mid)] bg-background px-3 py-2 hover:border-[var(--wire-strong)] focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
            :data-testid="`library-${library.id}`"
          >
            <LibraryBig class="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
            <span class="min-w-0 flex-1">
              <span class="block truncate font-heading text-xs font-semibold">{{ library.name }}</span>
              <span class="block truncate font-mono text-[10px] text-[var(--text-secondary)]">{{ library.root_path }}</span>
            </span>
            <span class="shrink-0 text-[10px] text-[var(--text-secondary)]">
              {{ library.last_scan_status === 'completed' ? '已扫描' : library.last_scan_status === 'failed' ? '扫描失败' : '未扫描' }}
            </span>
          </RouterLink>
          <Button
            variant="ghost"
            size="icon"
            :aria-label="`编辑 ${library.name}`"
            :data-testid="`edit-${library.id}`"
            @click="openEdit(library.id)"
          >
            <Pencil class="size-3.5" />
          </Button>
        </li>
      </ul>
    </div>

    <LibraryManager
      v-if="managerOpen"
      :open="managerOpen"
      :library="editingLibrary"
      :saving="saving"
      @close="managerOpen = false"
      @save="saveLibrary"
      @remove="removeLibrary"
    />
  </section>
</template>
