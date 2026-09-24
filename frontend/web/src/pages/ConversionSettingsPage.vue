<script setup lang="ts">
import { computed, watch } from 'vue'
import { onBeforeRouteLeave, useRoute, useRouter } from 'vue-router'
import { Button } from '@/components/ui/button'
import CommonSettingsForm from '@/features/worksets/CommonSettingsForm.vue'
import { classifierTagLibraryQueryOptions } from '@/queries/worksets'
import { useApiClient } from '@/lib/api/client'
import { useQuery } from '@tanstack/vue-query'
import { useCurrentConversion } from '@/composables/use-operation-context'
import { useWorksetEditorStore } from '@/stores/workset-editor'

/**
 * Common conversion settings. Each unit shows how many members it reaches, so
 * a common change never silently affects folders the user is not looking at
 *. Applying reaches further than this record: the common settings are stored
 * deployment-wide and pushed to every other task, which the page states before
 * the button (ADR 0010 §2). Generation and execution stay on the operation list
 * page.
 */
const route = useRoute()
const router = useRouter()
const editor = useWorksetEditorStore()
const libraryId = computed(() => (route.params.libraryId as string) || '')
const { worksetId, workspace, applySession } = useCurrentConversion(libraryId)
const draft = workspace.draft
const api = useApiClient()
// 恢复默认 restores the deployment's literal tags, so the form needs them.
const { data: tagLibrary } = useQuery(classifierTagLibraryQueryOptions(api))
const defaultTags = computed(() => tagLibrary.value?.default_tags ?? [])

watch(
  [draft, worksetId],
  () => {
    if (!draft.value || !worksetId.value) return
    const current = editor.session
    if (current && current.worksetId === worksetId.value && current.target.kind === 'common') return
    editor.open({
      worksetId: worksetId.value,
      operation: 'conversion',
      target: { kind: 'common' },
      baseVersion: draft.value.version,
      baseDocument: draft.value.document,
    })
  },
  { immediate: true },
)

const dirty = computed(() => editor.isDirty)
const session = computed(() => editor.session)

/** Per unit: members that inherit it and would therefore be affected. */
const affected = computed(() => {
  const document = draft.value?.document
  const members = workspace.workset.value?.members ?? []
  if (!document) return { total: 0, excluded: 0 }
  let inheriting = 0
  let excluded = 0
  for (const member of members) {
    const record = document.members.find((m) => m.member_id === member.member_id)
    const overridesMode =
      record?.overrides?.mode !== undefined ||
      record?.overrides?.classifier_tags !== undefined ||
      record?.overrides?.matched !== undefined ||
      record?.overrides?.unmatched !== undefined
    void overridesMode
    inheriting++
    if (record?.excluded) excluded++
  }
  return { total: inheriting, excluded }
})

onBeforeRouteLeave(() => {
  if (!dirty.value) return true
  return window.confirm('有未应用的全局设置修改，确定放弃吗？')
})

async function apply() {
  await applySession()
}

function cancel() {
  if (dirty.value && !window.confirm('放弃未应用的修改？')) return
  editor.close()
  void router.push(listRoute.value)
}

/** Back to the conversion list: the library's record, at its own URL. */
const listRoute = computed(() => ({
  name: 'conversion' as const,
  params: { libraryId: libraryId.value },
}))
</script>

<template>
  <div class="mx-auto max-w-2xl p-3" data-testid="conversion-settings">
    <div class="mb-2 flex items-center gap-2">
      <RouterLink :to="listRoute" class="text-xs text-[var(--brand-ink)] hover:underline">← 转换列表</RouterLink>
      <h2 class="font-heading text-sm font-semibold">转换全局设置</h2>
    </div>

    <p class="rounded-md border border-[var(--brand-border)] bg-[var(--brand-weak)] p-2 text-[11px]" data-testid="global-settings-notice">
      转换全局设置影响所有任务：应用后所有任务的公共设置同步更新，已有计划需重新生成，执行前必须重新生成计划。孤立的只读任务不参与同步。
    </p>

    <p v-if="affected.total > 0" class="mt-2 text-[11px] text-[var(--text-muted)]" data-testid="affected-members">
      影响 {{ affected.total }} 个继承全局设置的文件夹<template v-if="affected.excluded > 0">
        （其中 {{ affected.excluded }} 个已从本操作排除：设置仍会更新）</template
      >。
    </p>
    <p v-if="session?.error" class="mt-2 text-[11px] text-[var(--danger-ink)]" role="alert">{{ session.error }}</p>

    <div v-if="draft" class="mt-3">
      <CommonSettingsForm :draft="draft.document" :default-tags="defaultTags" />
    </div>
    <p v-else class="text-xs text-[var(--text-muted)]">加载中…</p>

    <div class="mt-3 flex items-center gap-2">
      <Button size="sm" :disabled="!dirty || (session?.applying ?? false)" data-testid="apply-common" @click="apply">
        应用全局设置
      </Button>
      <Button size="sm" variant="ghost" @click="cancel">取消</Button>
    </div>
  </div>
</template>
