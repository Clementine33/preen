<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ChevronDown, ChevronRight, FileAudio, FileText, Folder, Music } from '@lucide/vue'
import type { TreeNode } from '@/lib/api/types'
import { createTreeModel, type TreeModel, type TreeModelNode } from './tree-model'

/**
 * The shared member tree: the same renderer for the workbench overview's file
 * page and a conversion member's file page. It reads and selects the files and
 * folders of ONE member directory; it knows nothing about plans, drafts or
 * revisions — the plan's own review view is a separate component fed by the
 * frozen plan (ADR 0001 §3). Actions on the selection live in the page's
 * toolbar, not on the rows (ADR 0022 §4).
 *
 * Selection is by member-relative path, which is also how every file
 * operation addresses its items; a selected folder stands for what is below
 * it. A symlinked path never appears as a manageable file: links are not
 * recursed into, and the backend refuses them.
 */
const props = withDefaults(
  defineProps<{
    root: TreeNode
    /** File modification is unavailable: refreshing, or the app is busy. */
    frozen?: boolean
    /** A refresh failed: the tree below is the cached one, and says so. */
    stale?: boolean
  }>(),
  { frozen: false, stale: false },
)

const emit = defineEmits<{
  selectionChange: [paths: string[]]
}>()

const model = ref<TreeModel>(createTreeModel(props.root))

// A re-read of the same member (refresh, media facts, a finished operation)
// keeps what the user opened and marked; only vanished entries drop.
watch(
  () => props.root,
  (root) => {
    const before = model.value.selectedPaths()
    model.value = createTreeModel(root, model.value)
    const after = model.value.selectedPaths()
    if (after.length !== before.length) emit('selectionChange', after)
  },
)

const rows = computed(() => model.value.getVisibleNodes())

function checked(node: TreeModelNode): boolean {
  return model.value.dirSelection(node.relPath) === 'checked'
}

function setSelected(node: TreeModelNode, on: boolean): void {
  model.value.select(node.relPath, on)
  emit('selectionChange', model.value.selectedPaths())
}

function onToggle(node: TreeModelNode, event: Event): void {
  setSelected(node, (event.target as HTMLInputElement).checked)
}

function onNameClick(node: TreeModelNode): void {
  if (props.frozen || model.value.isCovered(node.relPath)) return
  setSelected(node, !model.value.selected.has(node.relPath))
}

function formatBitrate(bitrate: number | null): string {
  if (!bitrate || bitrate <= 0) return ''
  return `${Math.round(bitrate / 1000)} kbps`
}

function formatBytes(size: number | null): string {
  if (size === null || size < 0) return ''
  if (size >= 1024 * 1024) return `${(size / (1024 * 1024)).toFixed(1)} MB`
  if (size >= 1024) return `${Math.round(size / 1024)} KB`
  return `${size} B`
}

function meta(node: TreeModelNode): string {
  return [formatBitrate(node.bitrate), formatBytes(node.size)].filter(Boolean).join(' · ')
}
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col" data-testid="member-tree">
    <p
      v-if="stale"
      class="border-b border-border bg-muted px-3 py-1.5 text-[11px] text-[var(--warning-ink,var(--text-secondary))]"
      data-testid="tree-stale"
    >
      未能刷新：下面是上次扫描的内容，可能已过期。
    </p>
    <div class="min-h-0 flex-1 overflow-auto">
      <div v-if="rows.length > 0" role="tree" aria-label="文件夹内容" class="pb-2">
        <div
          v-for="node in rows"
          :key="node.id"
          :data-testid="`tree-row-${node.relPath}`"
          :data-rel-path="node.relPath"
          :data-indent="node.depth - 1"
          role="treeitem"
          :aria-level="node.depth"
          :aria-expanded="node.type === 'dir' ? model.isExpanded(node.relPath) : undefined"
          :aria-selected="checked(node)"
          class="flex min-h-11 items-center gap-2 border-b border-border/60 py-1 pr-2 [--indent:12px] last:border-b-0 hover:bg-accent/40 @[641px]:min-h-9 @[641px]:[--indent:18px]"
          :class="{ 'bg-accent/30': model.selected.has(node.relPath) }"
          :style="{ paddingLeft: `calc(8px + ${node.depth - 1} * var(--indent))` }"
        >
          <button
            v-if="node.type === 'dir'"
            type="button"
            class="grid size-6 shrink-0 place-items-center rounded text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            :aria-label="`${model.isExpanded(node.relPath) ? '折叠' : '展开'} ${node.name}`"
            @click="model.toggleDir(node.relPath)"
          >
            <ChevronDown v-if="model.isExpanded(node.relPath)" class="size-3.5" />
            <ChevronRight v-else class="size-3.5" />
          </button>
          <span v-else class="size-6 shrink-0" aria-hidden="true" />

          <input
            :data-testid="`${node.type === 'file' ? 'file' : 'dir'}-checkbox-${node.relPath}`"
            type="checkbox"
            :checked="checked(node)"
            :indeterminate="node.type === 'dir' && model.dirSelection(node.relPath) === 'indeterminate'"
            :aria-label="`选择 ${node.name}`"
            :disabled="frozen || model.isCovered(node.relPath)"
            class="size-4 shrink-0 accent-[var(--ring)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            @change="onToggle(node, $event)"
          />

          <span
            v-if="node.type === 'dir'"
            class="grid size-4 shrink-0 place-items-center"
            :class="model.isExpanded(node.relPath) ? 'text-[var(--brand)]' : 'text-muted-foreground'"
            aria-hidden="true"
          >
            <Folder class="size-4" />
          </span>
          <span v-else class="grid size-4 shrink-0 place-items-center text-muted-foreground" aria-hidden="true">
            <component :is="node.format === 'm4a' ? Music : node.format ? FileAudio : FileText" class="size-4" />
          </span>

          <span class="min-w-0 flex-1 cursor-pointer" @click="onNameClick(node)">
            <span
              class="line-clamp-2 font-heading text-xs font-semibold [overflow-wrap:anywhere]"
              :title="node.name"
            >
              {{ node.name }}
            </span>
            <span
              v-if="node.type === 'file' && meta(node)"
              class="block font-mono text-[10px] text-muted-foreground"
            >
              {{ meta(node) }}
            </span>
          </span>
        </div>
      </div>

      <div v-else class="grid min-h-40 place-items-center px-4 text-center">
        <div class="max-w-xs">
          <Music class="mx-auto size-6 text-muted-foreground" />
          <h3 class="mt-2 font-heading text-sm font-semibold">这个文件夹是空的</h3>
          <p class="mt-1 text-xs leading-5 text-muted-foreground">
            扫描后这里会显示目录内容。
          </p>
        </div>
      </div>
    </div>
  </div>
</template>
