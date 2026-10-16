import { computed, onBeforeUnmount, onMounted, ref, toValue, type MaybeRefOrGetter } from 'vue'
import { onBeforeRouteLeave } from 'vue-router'
import { sameUnitValue } from '@/features/worksets/draft-intents'

/**
 * Edit state of one Task kind's common settings, shared by every settings
 * surface (conversion page, rename/metadata card).
 *
 * The page holds nothing but the edits: a field's value is its edit when it
 * has one, else the saved value. A field is `modified` only while its edit
 * differs from the saved value, so setting a field back to what is saved
 * leaves nothing to apply. 撤销 drops one edit; 恢复默认 writes every default
 * as edits, so it is applied or discarded like any other change.
 * Applying sends the edits to `save`, which owns the request and the cache;
 * a refusal is kept as text for the apply bar and the edits stay.
 */
export interface SettingsFormOptions<S extends object> {
  /** The saved settings as the form reads them; undefined while loading. */
  saved: MaybeRefOrGetter<S | undefined>
  /** The value a fresh install starts from; undefined while unknown. */
  defaults: MaybeRefOrGetter<S | undefined>
  /** Persist the edited fields over the saved settings. */
  save: (edits: Partial<S>) => Promise<unknown>
  /** Names the kind in refusals: 转换, 重命名, 元数据刷新. */
  label: string
  /** The saved copy moved on the server: reload it. Edits stay. */
  onConflict?: () => void
}

export function useSettingsForm<S extends object>(options: SettingsFormOptions<S>) {
  const edits = ref<Partial<S>>({}) as { value: Partial<S> }
  const applying = ref(false)
  const error = ref<string | null>(null)

  const saved = () => toValue(options.saved)

  function modified<K extends keyof S>(key: K): boolean {
    const current = saved()
    return current !== undefined && key in edits.value && !sameUnitValue(edits.value[key], current[key])
  }

  /** The value the field shows: its pending edit, else the saved one. */
  function value<K extends keyof S>(key: K): S[K] {
    return (key in edits.value ? edits.value[key] : saved()?.[key]) as S[K]
  }

  function set<K extends keyof S>(key: K, next: S[K]) {
    error.value = null
    edits.value = { ...edits.value, [key]: next }
  }

  function undo<K extends keyof S>(key: K) {
    const { [key]: _dropped, ...rest } = edits.value
    edits.value = rest as Partial<S>
  }

  /** 恢复默认 is only useful while some field shows a value other than its default. */
  const canRestoreDefaults = computed(() => {
    const defaults = toValue(options.defaults)
    return (
      saved() !== undefined &&
      defaults !== undefined &&
      (Object.keys(defaults) as (keyof S)[]).some((key) => !sameUnitValue(value(key), defaults[key]))
    )
  })

  /** Every field back to its default, as unsaved edits the apply bar still owns. */
  function restoreDefaults() {
    const defaults = toValue(options.defaults)
    if (!defaults) return
    error.value = null
    edits.value = { ...edits.value, ...defaults }
  }

  const dirty = computed(() => (Object.keys(edits.value) as (keyof S)[]).some(modified))

  function discard() {
    edits.value = {}
    error.value = null
  }

  async function apply(): Promise<boolean> {
    if (!dirty.value || applying.value) return false
    const pending = Object.fromEntries(
      (Object.keys(edits.value) as (keyof S)[]).filter(modified).map((key) => [key, edits.value[key]]),
    ) as Partial<S>
    applying.value = true
    error.value = null
    try {
      await options.save(pending)
      edits.value = {}
      return true
    } catch (failure) {
      error.value = describeSaveFailure(failure, options.label)
      if ((failure as { code?: string }).code === 'VERSION_CONFLICT') options.onConflict?.()
      return false
    } finally {
      applying.value = false
    }
  }

  // Leaving with unapplied changes asks first: in-app by route, and by closing
  // the tab.
  onBeforeRouteLeave(() => !dirty.value || window.confirm('有未应用的设置修改，确定放弃吗？'))
  const warnBeforeUnload = (event: BeforeUnloadEvent) => {
    if (dirty.value) event.preventDefault()
  }
  onMounted(() => window.addEventListener('beforeunload', warnBeforeUnload))
  onBeforeUnmount(() => window.removeEventListener('beforeunload', warnBeforeUnload))

  return { value, set, modified, undo, canRestoreDefaults, restoreDefaults, dirty, discard, apply, applying, error }
}

/** Why an apply was refused, in words the apply bar can show next to the button. */
export function describeSaveFailure(failure: unknown, label: string): string {
  const { code, message } = failure as { code?: string; message?: string }
  switch (code) {
    case 'COMMON_SETTINGS_BUSY':
      return `另一条${label}记录正在生成或执行：设置未保存，请等它结束后重试`
    case 'COMMON_SETTINGS_UNREADABLE':
      return `有一条${label}记录的草稿无法读取：设置未保存，请重建该记录后重试`
    case 'VERSION_CONFLICT':
      return '设置已在别处被修改，已载入最新值；你的修改仍保留，确认后再次应用'
    default:
      return message ?? '保存设置失败'
  }
}
