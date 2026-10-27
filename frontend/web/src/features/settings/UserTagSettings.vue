<script setup lang="ts">
import type { WorkUserTag } from '@/lib/api/types'
import { computed, nextTick, ref } from 'vue'
import { Button } from '@/components/ui/button'
import Modal from '@/components/ui/modal/Modal.vue'
import { useToast } from '@/composables/use-toast'
import { errorDetails } from '@/lib/api/error'
import { useCreateUserTag, useDeleteUserTag, useRenameUserTag, useTagFacets, useUserTags } from '@/queries/works'

/**
 * The whole user-tag vocabulary, including tags no work carries yet (it reads
 * `GET /user-tags`, never the wall's tag index). Create is idempotent by name,
 * a rename onto a held name is refused rather than merged, and a delete takes
 * only the tag and its relations. A failed write keeps what was typed.
 */
const { query, tags } = useUserTags()
// Work counts come from the unscoped tag index; a tag it omits has none.
const { facets } = useTagFacets({})
const counts = computed(() => new Map(facets.value.user_tags.map(facet => [facet.id, facet.work_count])))
const sorted = computed(() => [...tags.value].sort((a, b) => a.name.localeCompare(b.name, 'zh-CN')))

const toast = useToast()
const create = useCreateUserTag()
const rename = useRenameUserTag()
const remove = useDeleteUserTag()

const newName = ref('')
const createError = ref('')
async function submitCreate(): Promise<void> {
  const name = newName.value.trim()
  if (!name)
    return
  createError.value = ''
  const existed = tags.value.some(tag => tag.name === name)
  try {
    await create.mutateAsync(name)
    newName.value = ''
    toast.show({
      text: existed
        ? `标签「${name}」已存在`
        : `已新建「${name}」`,
    })
  }
  catch (error) {
    createError.value = errorDetails(error).message
  }
}

const editingId = ref<string | null>(null)
const editName = ref('')
const editError = ref('')
const editInput = ref<HTMLInputElement[] | null>(null)
async function startRename(tag: WorkUserTag): Promise<void> {
  editingId.value = tag.id
  editName.value = tag.name
  editError.value = ''
  await nextTick()
  editInput.value?.[0]?.select()
}
async function submitRename(tag: WorkUserTag): Promise<void> {
  const name = editName.value.trim()
  if (!name)
    return
  if (name === tag.name)
    return void (editingId.value = null)
  editError.value = ''
  try {
    await rename.mutateAsync({ id: tag.id, name })
    editingId.value = null
  }
  catch (error) {
    const details = errorDetails(error)
    editError.value = details.code === 'USER_TAG_EXISTS'
      ? `「${name}」已被另一个标签使用。标签不会自动合并。`
      : details.message
  }
}

const deleting = ref<WorkUserTag | null>(null)
const deleteError = ref('')
function askDelete(tag: WorkUserTag): void {
  deleting.value = tag
  deleteError.value = ''
}
async function confirmDelete(): Promise<void> {
  const tag = deleting.value
  if (!tag)
    return
  try {
    await remove.mutateAsync(tag.id)
    deleting.value = null
    toast.show({ text: `已删除标签「${tag.name}」` })
  }
  catch (error) {
    deleteError.value = errorDetails(error).message
  }
}
const deletingCount = computed(() => (deleting.value
  ? (counts.value.get(deleting.value.id) ?? 0)
  : 0))
</script>

<template>
  <div data-testid="user-tag-settings">
    <form class="flex flex-wrap items-start gap-2" @submit.prevent="submitCreate">
      <input
        v-model="newName"
        class="field-input h-11! min-w-0 flex-1 text-[16px]! rail:h-[30px]! rail:text-[length:var(--fs-s)]!"
        placeholder="新标签名称"
        aria-label="新标签名称"
        autocomplete="off"
        data-testid="user-tag-new"
        @input="createError = ''"
      >
      <Button type="submit" class="h-11 rail:h-[30px]" size="sm" variant="outline" :disabled="!newName.trim() || create.isPending.value" data-testid="user-tag-create">
        新建
      </Button>
      <p v-if="createError" class="w-full text-[11px] text-destructive" role="alert">新建失败：{{ createError }}</p>
    </form>

    <p v-if="query.isPending.value" class="py-3 text-xs text-muted-foreground">正在读取标签…</p>
    <div v-else-if="query.error.value" class="flex items-center gap-3 py-3 text-xs text-destructive">
      <span class="min-w-0 flex-1">标签读取失败：{{ errorDetails(query.error.value).message }}</span>
      <Button size="xs" variant="outline" @click="query.refetch()">重试</Button>
    </div>
    <p v-else-if="sorted.length === 0" class="py-3 text-xs text-muted-foreground" data-testid="user-tag-empty">
      还没有用户标签。在上面新建，或在作品详情页给作品加标签。
    </p>
    <template v-else>
      <div class="mt-3 flex items-center border-b border-[var(--wire-strong)] px-1 pb-1 text-[11px] text-[var(--text-secondary)]">
        <span class="flex-1">名称</span>
        <span class="w-16 text-right">作品数</span>
        <span class="w-[7.5rem]" aria-hidden="true" />
      </div>
      <ul class="divide-y divide-[var(--wire-fine)]">
        <li v-for="tag in sorted" :key="tag.id" :data-testid="`user-tag-${tag.id}`">
          <form
            v-if="editingId === tag.id"
            class="flex flex-wrap items-center gap-2 px-1 py-1.5"
            @submit.prevent="submitRename(tag)"
            @keydown.esc="editingId = null"
          >
            <input
              ref="editInput"
              v-model="editName"
              class="field-input h-11! min-w-0 flex-1 text-[16px]! rail:h-[30px]! rail:text-[length:var(--fs-s)]!"
              :aria-label="`重命名「${tag.name}」`"
              :aria-invalid="editError ? true : undefined"
              autocomplete="off"
              data-testid="user-tag-rename-input"
              @input="editError = ''"
            >
            <Button type="button" size="sm" variant="ghost" @click="editingId = null">取消</Button>
            <Button type="submit" size="sm" :disabled="!editName.trim() || rename.isPending.value" data-testid="user-tag-rename-submit">保存</Button>
            <p v-if="editError" class="w-full text-[11px] text-destructive" role="alert" data-testid="user-tag-rename-error">{{ editError }}</p>
          </form>
          <div v-else class="flex min-h-11 items-center px-1 rail:min-h-9">
            <span class="min-w-0 flex-1 truncate text-xs">{{ tag.name }}</span>
            <span class="w-16 text-right font-mono text-[11px] text-[var(--text-secondary)]">{{ counts.get(tag.id) ?? 0 }}</span>
            <span class="flex w-[7.5rem] justify-end gap-1">
              <Button size="xs" variant="ghost" class="text-[var(--text-secondary)]" :data-testid="`user-tag-rename-${tag.id}`" @click="startRename(tag)">重命名</Button>
              <Button size="xs" variant="ghost" class="text-[var(--text-secondary)] hover:text-[var(--danger-ink)]" :data-testid="`user-tag-delete-${tag.id}`" @click="askDelete(tag)">删除</Button>
            </span>
          </div>
        </li>
      </ul>
    </template>

    <Modal :open="deleting !== null" title="删除用户标签" @update:open="deleting = null">
      <p class="text-xs leading-relaxed">
        删除标签「{{ deleting?.name }}」？它会从{{ deletingCount > 0 ? ` ${deletingCount} 部` : '所有' }}作品上移除，作品本身和文件不会被删除。
      </p>
      <p v-if="deleteError" class="text-[11px] text-destructive" role="alert">删除失败：{{ deleteError }}</p>
      <div class="flex justify-end gap-2">
        <Button variant="ghost" size="sm" @click="deleting = null">取消</Button>
        <Button variant="destructive" size="sm" :disabled="remove.isPending.value" data-testid="user-tag-delete-confirm" @click="confirmDelete">删除</Button>
      </div>
    </Modal>
  </div>
</template>
