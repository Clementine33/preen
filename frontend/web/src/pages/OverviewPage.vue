<script setup lang="ts">
import { computed, ref, watch, watchEffect } from 'vue'
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import { RouterLink, RouterView, useRoute, useRouter } from 'vue-router'
import { AlertTriangle, Pencil, RefreshCw, ScanLine, Square } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import Modal from '@/components/ui/modal/Modal.vue'
import WorkbenchShell from '@/components/layout/WorkbenchShell.vue'
import WorkbenchNav from '@/features/worksets/WorkbenchNav.vue'
import { settingsEditBlockedReason } from '@/features/worksets/workbench-nav'
import ScanProgressBar from '@/features/libraries/ScanProgressBar.vue'
import DirList from '@/features/libraries/DirList.vue'
import LibraryManager from '@/features/libraries/LibraryManager.vue'
import BatchPlanBar, { type TaskTarget } from '@/features/libraries/BatchPlanBar.vue'
import { useLibraryScan } from '@/composables/use-library-scan'
import { useApiClient } from '@/lib/api/client'
import { errorDetails } from '@/lib/api/error'
import { rootPathIdentityKey } from '@/lib/root-path-identity'
import { dirsQueryOptions, useLibraryList } from '@/queries/libraries'
import { updateLibraryMutationOptions, deleteLibraryMutationOptions } from '@/queries/libraries'
import { createCurrentRecordMutationOptions, currentRecordQueryOptions } from '@/queries/worksets'
import { useLibraryUiStore } from '@/stores/library-ui'
import { useScanStore } from '@/stores/scan'
import {
  CONVERSION,
  type LibraryDir,
  type SkippedFolder,
  type SkippedReason,
} from '@/lib/api/types'

/**
 * The workbench overview of one library (ADR 0001 §1, §2).
 *
 * The library IS the workbench: entering it lists every direct child directory
 * the last scan saw — empty ones and ones without audio included — and that is
 * where the shared batch is chosen. Choosing folders creates the library's
 * workset explicitly; browsing alone creates nothing.
 */
const route = useRoute()
const router = useRouter()
const api = useApiClient()
const queryClient = useQueryClient()
const ui = useLibraryUiStore()
const scan = useScanStore()
const libraryScan = useLibraryScan()

const libraryId = computed(() => (route.params.libraryId as string) || '')
const filesOpen = computed(() => route.name === 'overview-files')

const { query: librariesQuery, librariesData, activeLibrary } = useLibraryList()
const rootIdentity = computed(() => {
  const library = librariesData.value.find((item) => item.id === libraryId.value)
  if (library) return rootPathIdentityKey(library.root_path)
  if (librariesQuery.isSuccess.value || librariesQuery.error.value) return 'unresolved-root'
  return null
})

watch(
  libraryId,
  (id) => {
    if (id) ui.setActiveLibrary(id)
  },
  { immediate: true },
)

const dirsQuery = useQuery(() => dirsQueryOptions(api, libraryId.value, rootIdentity.value))
watchEffect(() => {
  if (dirsQuery.data.value) ui.reconcileDirs(libraryId.value, dirsQuery.data.value)
})
const dirs = computed(() => dirsQuery.data.value ?? [])
const dirsPending = computed(() => dirsQuery.isPending.value)
const dirsError = computed(() => {
  const error = dirsQuery.error.value
  return error ? errorDetails(error) : null
})

// One folder batch is shared by every task page.
const recordQuery = useQuery(() => currentRecordQueryOptions(api, libraryId.value, CONVERSION))
const record = computed(() => recordQuery.data.value?.workset ?? null)
const conversionOperation = computed(
  () => record.value?.operations.find((operation) => operation.operation_type === CONVERSION) ?? null,
)
// The sidebar disables 转换全局设置 for the same reasons the conversion page
// does: this page shares the navigation, so it shares its one source of truth.
const settingsBlockedReason = computed(() => settingsEditBlockedReason(conversionOperation.value))

// ---- scope → record ---------------------------------------------------

const managerOpen = ref(false)
const savingLibrary = ref(false)
const confirmOpen = ref(false)
const createError = ref<string | null>(null)
const skipped = ref<SkippedFolder[]>([])
let idempotencyKey = ''

const createMutation = useMutation(() => createCurrentRecordMutationOptions(api, queryClient, libraryId.value, CONVERSION))
const creating = computed(() => createMutation.isPending.value)

const SKIP_TEXT: Record<SkippedReason, string> = {
  no_audio: '没有音频，将跳过',
  missing: '目录不存在',
  recovery_dir: '回收目录不作为成员',
  duplicate: '重复选择',
  invalid_path: '路径不合法',
  not_work_root: '不是扫描识别的作品根目录',
}

// ---- search & order ---------------------------------------------------

type DirSort = 'rjid' | 'added'
const SORTS: { value: DirSort; label: string }[] = [
  { value: 'rjid', label: 'RJ号' },
  { value: 'added', label: '最近添加' },
]
const search = ref('')
const sort = ref<DirSort>('rjid')

function byPath(a: LibraryDir, b: LibraryDir) {
  return a.rel_path < b.rel_path ? -1 : a.rel_path > b.rel_path ? 1 : 0
}
/** Descending only. A folder with no work code sorts after every coded one. */
function compareDirs(a: LibraryDir, b: LibraryDir): number {
  if (sort.value === 'rjid') {
    if (a.work_code !== b.work_code) {
      if (a.work_code === null) return 1
      if (b.work_code === null) return -1
      return a.work_code < b.work_code ? 1 : -1
    }
  } else if (a.added_at !== b.added_at) {
    return a.added_at < b.added_at ? 1 : -1
  }
  return byPath(a, b)
}
const sortedDirs = computed(() => [...dirs.value].sort(compareDirs))
const shownDirs = computed(() => {
  const query = search.value.trim().toLowerCase()
  if (!query) return sortedDirs.value
  return sortedDirs.value.filter(
    (dir) => dir.name.toLowerCase().includes(query) || dir.rel_path.toLowerCase().includes(query),
  )
})
const allShownSelected = computed(
  () => shownDirs.value.length > 0 && shownDirs.value.every((dir) => ui.selectedDirPaths.includes(dir.rel_path)),
)

/** The selection as the caller sees it, so the confirmation names it. */
const selectedDirs = computed(() => dirs.value.filter((dir) => ui.selectedDirPaths.includes(dir.rel_path)))
/** A directory the conversion will skip: it is selectable, but holds no audio. */
const selectedWithoutAudio = computed(() => selectedDirs.value.filter((dir) => dir.audio_file_count === 0))

/** The task to open once the selection is the batch. */
let pendingTarget: TaskTarget = 'conversion'

function startEnter(target: TaskTarget) {
  pendingTarget = target
  if (ui.selectedDirPaths.length === 0) return
  skipped.value = []
  createError.value = null
  idempotencyKey = crypto.randomUUID()
  if (record.value) {
    // Replacing deletes member edits, plan and execution results; common
    // settings remain available to seed the new record.
    confirmOpen.value = true
    return
  }
  void submit()
}

async function submit() {
  createError.value = null
  try {
    const response = await createMutation.mutateAsync({
      request: {
        // The order on screen is the batch's order: every task lists its
        // members as stored, so none of them sorts on its own. Taken from the
        // unfiltered list so a search never drops a selected folder.
        folder_paths: sortedDirs.value
          .filter((dir) => ui.selectedDirPaths.includes(dir.rel_path))
          .map((dir) => dir.rel_path),
        // The record the caller saw as current: a concurrent replace is a
        // conflict, not an overwrite.
        expected_current_id: record.value?.workset_id,
      },
      idempotencyKey,
    })
    skipped.value = response.skipped
    confirmOpen.value = false
    ui.clearSelection()
    // A skipped folder is reported here first; the task can be opened from the card.
    if (response.skipped.length === 0) {
      await router.push({ name: pendingTarget, params: { libraryId: libraryId.value } })
    }
  } catch (error) {
    const apiError = error as { code?: string; message?: string; details?: string[] }
    skipped.value = (apiError.details ?? []).map((detail) => {
      const [path, reason] = detail.split(': ')
      return { path, reason: (reason ?? '') as SkippedReason }
    })
    createError.value = apiError.message ?? '无法创建处理记录'
    confirmOpen.value = false
  }
}

// ---- library management ------------------------------------------------

const updateLibraryMutation = useMutation(updateLibraryMutationOptions(api, queryClient))
const deleteLibraryMutation = useMutation(deleteLibraryMutationOptions(api, queryClient))

async function saveLibrary(input: { name: string; root_path: string }) {
  if (!activeLibrary.value) return
  savingLibrary.value = true
  try {
    // A root change is refused by the backend while a record exists; the
    // dialog surfaces that refusal as it is.
    await updateLibraryMutation.mutateAsync({ id: activeLibrary.value.id, input })
    managerOpen.value = false
  } catch {
    // The mutation error is shown in the dialog.
  } finally {
    savingLibrary.value = false
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
  savingLibrary.value = true
  try {
    await deleteLibraryMutation.mutateAsync(id)
    managerOpen.value = false
    await router.push({ name: 'worksets' })
  } catch {
    // Keep the dialog open and expose the mutation error in it.
  } finally {
    savingLibrary.value = false
  }
}

const managementError = computed(() => {
  const error = updateLibraryMutation.error.value ?? deleteLibraryMutation.error.value
  return error ? errorDetails(error) : null
})

// ---- scanning ---------------------------------------------------------

const scanningThisLibrary = computed(() => scan.status === 'scanning' && scan.libraryId === libraryId.value)

async function runScan() {
  if (!libraryId.value) return
  await libraryScan.start(libraryId.value)
}

// ---- misc -------------------------------------------------------------

/** Opening a member: its files, in this same workbench. The address
 * carries the directory's identity, so the folder's name stays out of it. */
async function openMember(dirId: string) {
  await router.push({
    name: 'overview-files',
    params: { libraryId: libraryId.value, dirId },
  })
}

function onSelectAll(value: boolean) {
  if (!value) {
    ui.clearSelection()
    return
  }
  for (const dir of shownDirs.value) ui.setDirSelected(dir.rel_path, true, dirs.value)
}

/** The page in view, named as the navigation names it (ADR 0001 §1; ADR 0003 §1). */
const pageTitle = computed(() => (filesOpen.value ? '文件' : '概览与成员'))
const libraryRoot = computed(() => activeLibrary.value?.root_path ?? '')
</script>

<template>
  <WorkbenchShell
    nav-title="工作台"
    :detail-column="false"
    :context-title="activeLibrary?.name ?? '…'"
    :context-page="pageTitle"
    :context-back-to="filesOpen ? { name: 'workbench-overview', params: { libraryId } } : { name: 'worksets' }"
    :context-back-label="filesOpen ? '概览与成员' : '工作集列表'"
  >
    <template #header>
      <nav aria-label="面包屑" class="flex min-w-0 items-center gap-1.5 text-xs" data-testid="overview-breadcrumb">
        <RouterLink to="/worksets" class="rounded px-1 py-0.5 text-[var(--text-secondary)] hover:bg-muted">
          工作集
        </RouterLink>
        <span aria-hidden="true" class="text-[var(--text-muted)]">/</span>
        <RouterLink
          :to="{ name: 'workbench-overview', params: { libraryId } }"
          class="max-w-40 truncate rounded px-1 py-0.5 font-medium hover:bg-muted"
        >
          {{ activeLibrary?.name ?? '…' }}
        </RouterLink>
        <template v-if="filesOpen">
          <span aria-hidden="true" class="text-[var(--text-muted)]">/</span>
          <span class="px-1 font-medium text-[var(--text-secondary)]" aria-current="page">文件</span>
        </template>
      </nav>
    </template>

    <template #nav>
      <WorkbenchNav :library-id="libraryId" :settings-blocked-reason="settingsBlockedReason" />
    </template>

    <template #main>
      <div class="relative flex min-h-0 flex-1 flex-col">
      <!-- Files view: a member of this library, addressed by its relative
           path. It covers the list rather than replacing it, so the list keeps
           its layout — and with it the selection, the filters and the scroll
           position. -->
      <div v-if="filesOpen" class="absolute inset-0 z-10 flex min-h-0 flex-col substrate">
        <RouterView />
      </div>

      <section class="flex min-h-0 flex-1 flex-col overflow-y-auto" data-testid="overview">
        <div class="mx-auto flex w-full max-w-5xl min-h-0 flex-1 flex-col p-4">
          <!-- Library identity, its scan state and the library-level actions
              : management and scanning live here, not in a second page. -->
          <div class="flex flex-wrap items-start gap-3">
            <div class="min-w-0">
              <h1 class="truncate font-heading text-base font-semibold">
                {{ activeLibrary?.name ?? '…' }}
              </h1>
              <p class="truncate font-mono text-[11px] text-[var(--text-muted)]" :title="libraryRoot">
                {{ libraryRoot }}
              </p>
            </div>
            <div class="ml-auto flex items-center gap-2">
              <Button
                v-if="activeLibrary"
                size="xs"
                variant="outline"
                data-testid="scan-button"
                :disabled="scanningThisLibrary"
                @click="runScan"
              >
                <ScanLine class="size-3.5" />
                {{ scanningThisLibrary ? '扫描中…' : '扫描' }}
              </Button>
              <Button v-if="scanningThisLibrary" size="xs" variant="ghost" data-testid="cancel-scan" @click="libraryScan.cancel">
                <Square class="size-3" />
                取消
              </Button>
              <!-- Library management stays reachable from the workbench:
                   the entry lives here, not only on the selection page. -->
              <Button
                v-if="activeLibrary"
                size="xs"
                variant="ghost"
                data-testid="edit-library"
                @click="managerOpen = true"
              >
                <Pencil class="size-3.5" />
                媒体库
              </Button>
            </div>
          </div>

          <ScanProgressBar
            v-if="scan.libraryId === libraryId"
            class="mt-2"
            :status="scan.status"
            :files-scanned="scan.filesScanned"
            :dirs-scanned="scan.dirsScanned"
            :error-code="scan.errorCode"
            :error-message="scan.errorMessage"
          />

          <p
            v-if="createError"
            class="mt-3 rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-[11px] text-destructive"
            role="alert"
            data-testid="create-error"
          >
            {{ createError }}
          </p>
          <ul v-if="skipped.length" class="mt-3 space-y-0.5" data-testid="skipped-report">
            <li v-for="item in skipped" :key="item.path" class="text-[11px] text-[var(--text-secondary)]">
              <span class="font-mono">{{ item.path }}</span> — {{ SKIP_TEXT[item.reason] ?? item.reason }}
            </li>
          </ul>

          <!-- Members: every direct child directory. Selecting some is what a
               conversion scope is built from. -->
          <section class="mt-4 flex min-h-0 flex-1 flex-col">
            <div class="flex flex-wrap items-center gap-2">
              <h2 class="text-xs font-semibold text-[var(--text-secondary)]">
                文件夹（{{ search.trim() ? `${shownDirs.length} / ${dirs.length}` : dirs.length }}）
              </h2>
              <input
                v-model="search"
                type="search"
                placeholder="搜索文件夹"
                aria-label="搜索文件夹"
                data-testid="dir-search"
                class="h-7 min-w-0 flex-1 rounded-md border border-[var(--control-border)] bg-background px-2 text-xs focus-visible:ring-2 focus-visible:ring-[var(--brand)] focus-visible:outline-none @[641px]:max-w-xs"
              />
              <div class="flex items-center gap-1" role="group" aria-label="排序（降序）">
                <Button
                  v-for="item in SORTS"
                  :key="item.value"
                  size="xs"
                  :variant="sort === item.value ? 'secondary' : 'ghost'"
                  :aria-pressed="sort === item.value"
                  :data-testid="`sort-${item.value}`"
                  @click="sort = item.value"
                >
                  {{ item.label }}
                </Button>
              </div>
            </div>

            <p v-if="dirsPending" class="mt-1 text-[11px] text-[var(--text-muted)]">正在读取文件夹…</p>
            <p v-else-if="dirsError" class="mt-1 flex items-center gap-1.5 text-[11px] text-destructive">
              <AlertTriangle class="size-3.5" />{{ dirsError.code }} {{ dirsError.message }}
              <Button size="xs" variant="outline" @click="dirsQuery.refetch()">
                <RefreshCw class="size-3" />重试
              </Button>
            </p>
            <p v-else-if="dirs.length === 0" class="mt-1 text-[11px] text-[var(--text-muted)]" data-testid="no-dirs">
              还没有扫描结果。先扫描媒体库，这里会列出根目录下的所有文件夹。
            </p>

            <!-- The list owns the rest of the page: it is the thing being read,
                 and it scrolls internally. `min-h-80` is the floor a short
                 viewport keeps; below it the page itself scrolls. -->
            <div
              v-else
              class="mt-1 flex min-h-80 flex-1 flex-col overflow-hidden rounded-lg border border-border bg-card"
            >
              <DirList
                :dirs="shownDirs"
                :selected-paths="ui.selectedDirPaths"
                :all-selected="allShownSelected"
                @select="(relPath, selected) => ui.setDirSelected(relPath, selected, dirs)"
                @select-all="onSelectAll"
                @open="openMember"
              />
            </div>
          </section>
        </div>
      </section>
      </div>
    </template>

    <!-- The scope bar belongs to the selection and lives in the page's own
         column, so it is reachable at every container tier. -->
    <template #bottom>
      <BatchPlanBar
        v-if="!filesOpen && ui.selectedDirPaths.length > 0"
        :selected-count="ui.selectedDirPaths.length"
        :loading="creating"
        :replacing="Boolean(record)"
        :audio-less-count="selectedWithoutAudio.length"
        @clear="ui.clearSelection()"
        @enter="startEnter"
      />
    </template>
  </WorkbenchShell>

  <!-- The library's own management dialog (rename, root, delete): reachable
       from the workbench, and portalled so it works at every tier. -->
  <LibraryManager
    v-if="managerOpen"
    :open="managerOpen"
    :library="activeLibrary"
    :saving="savingLibrary"
    @close="managerOpen = false"
    @save="saveLibrary"
    @remove="removeLibrary"
  />
  <p v-if="managementError" class="sr-only" role="alert">{{ managementError.message }}</p>

  <!-- The replace confirmation is a page-level decision, not a carrier: it is
       portalled, so it reaches the user at every container tier. -->
  <Modal
    :open="confirmOpen"
    title="替换当前工作集批次？"
    @update:open="confirmOpen = false"
  >
    <p class="text-xs">
      这个媒体库已有一个工作集批次。替换会删除所有 Task 的成员设置、当前计划与执行结果，且无法恢复；全局设置会保留。
    </p>
    <p class="text-[11px] text-[var(--text-muted)]">文件本身不会被修改。</p>
    <div class="flex justify-end gap-2">
      <Button variant="ghost" size="sm" @click="confirmOpen = false">取消</Button>
      <Button size="sm" :disabled="creating" data-testid="confirm-replace" @click="submit">替换</Button>
    </div>
  </Modal>
</template>
