import { onScopeDispose } from 'vue'

/**
 * The app's one keyboard dispatcher. Components never listen to `keydown`
 * themselves: they register handlers for the shortcuts named in `KEYS`, which is
 * also the list Settings shows. The guards live here once — a text field, an open
 * menu or a dialog keeps its keys; Enter and Space on a link or button keep their
 * native meaning; anything held with Ctrl/Meta/Alt is left to the browser.
 *
 * Two layers: page shortcuts are checked first, the player's after them (the
 * `player` group), and the first handler that matches stops dispatch. Modifiers
 * match exactly (`→` and `Shift+→` differ). While a modal `<dialog>` is open the
 * page layer is suspended; the player layer stays.
 */

export type KeyGroup = 'player' | 'wall' | 'detail' | 'collections'

interface KeyDescriptor {
  group: KeyGroup
  /** `[Shift+]Key`, `Key` being a `KeyboardEvent.key` (` ` is spelled `Space`). */
  keys: string[]
  text: string
}

export const GROUP_LABELS: Record<KeyGroup, string> = {
  player: '播放器',
  wall: '作品墙',
  detail: '作品详情',
  collections: '合集',
}

const ARROWS = ['ArrowLeft', 'ArrowUp', 'ArrowDown', 'ArrowRight']
const key = (group: KeyGroup, keys: string[], text: string): KeyDescriptor => ({ group, keys, text })

export const KEYS = {
  'player.toggle': key('player', ['Space'], '播放 / 暂停'),
  'player.seek': key('player', ['Shift+ArrowLeft', 'Shift+ArrowRight'], '快退 / 快进 10 秒'),
  'player.volume': key('player', ['Shift+ArrowUp', 'Shift+ArrowDown'], '音量 +10% / −10%'),
  'player.mute': key('player', ['M'], '静音'),
  'player.step': key('player', [',', '.'], '上一首 / 下一首'),
  'player.lyrics': key('player', ['L'], '悬浮歌词'),
  'wall.move': key('wall', ARROWS, '移动选择'),
  'wall.page': key('wall', ['PageUp', 'PageDown'], '上一页 / 下一页'),
  'wall.play': key('wall', ['P'], '播放所选作品'),
  'wall.open': key('wall', ['Enter'], '打开所选作品'),
  'detail.step': key('detail', ['ArrowLeft', 'ArrowRight'], '上一张 / 下一张插画'),
  'detail.flip': key('detail', ['I'], '翻转插画'),
  'detail.close': key('detail', ['Escape'], '收起底部机库'),
  'detail.cursor': key('detail', ['ArrowUp', 'ArrowDown'], '在机库列表中移动光标'),
  'detail.activate': key('detail', ['Enter'], '执行光标行（等同点击）'),
  'detail.parent': key('detail', ['Backspace'], '文件页返回上级目录'),
  'collections.move': key('collections', ARROWS.slice(1, 3), '移动选择'),
  'collections.page': key('collections', ['PageUp', 'PageDown'], '上移 / 下移 5 项'),
  'collections.ends': key('collections', ['Home', 'End'], '第一项 / 最后一项'),
  'collections.open': key('collections', ['Enter'], '打开所选合集或作品'),
  'collections.redraw': key('collections', ['R'], '重抽封面'),
  'collections.close': key('collections', ['Escape'], '收起展开项 / 结束编辑'),
  'collections.remove': key('collections', ['Delete'], '编辑时移除所选作品'),
} satisfies Record<string, KeyDescriptor>

export type KeyId = keyof typeof KEYS

/** A handler returns `false` to decline the key, which then falls through to the next. */
type Handler = (event: KeyboardEvent) => void | false
interface Registration {
  handlers: Partial<Record<KeyId, Handler>>
  when?: () => boolean
}

const registrations: Registration[] = []
const isPlayer = (id: KeyId): boolean => KEYS[id].group === 'player'
/** Each key a registration binds, qualified by its layer. */
const bound = (handlers: Registration['handlers']): string[] =>
  (Object.keys(handlers) as KeyId[]).flatMap((id) => KEYS[id].keys.map((combo) => `${isPlayer(id) ? 'player' : 'page'}:${combo}`))

/** Single characters match case-insensitively (shift is in the character); named keys match shift exactly. */
function matches(combo: string, event: KeyboardEvent): boolean {
  const shift = combo.startsWith('Shift+')
  const name = shift ? combo.slice(6) : combo === 'Space' ? ' ' : combo
  if (name.length === 1) return event.key.toLowerCase() === name.toLowerCase()
  return event.key === name && event.shiftKey === shift
}

/** A text field or an open menu or dialog keeps every key it receives. */
function ownsKeys(target: EventTarget | null): boolean {
  return target instanceof HTMLElement && target.closest('[data-own-keys], [role="dialog"], input, textarea, select, [contenteditable]') !== null
}

function onKeydown(event: KeyboardEvent): void {
  if (ownsKeys(event.target) || event.ctrlKey || event.metaKey || event.altKey || event.isComposing) return
  // Enter and Space already mean something on a link or a button.
  if ((event.key === 'Enter' || event.key === ' ') && event.target instanceof HTMLElement && event.target.closest('a, button')) return
  const modal = document.querySelector('dialog[open]') !== null
  for (const player of [false, true]) {
    if (!player && modal) continue
    for (const { handlers, when } of registrations) {
      if (when && !when()) continue
      for (const [id, run] of Object.entries(handlers) as [KeyId, Handler][]) {
        if (isPlayer(id) !== player || !KEYS[id].keys.some((combo) => matches(combo, event))) continue
        if (run(event) === false) continue
        event.preventDefault()
        return
      }
    }
  }
}

/**
 * Register shortcuts for the life of the calling scope. `when` pauses them
 * (e.g. while a drawer is shut). Binding the same key twice in one layer throws in development.
 */
export function useKeymap(handlers: Registration['handlers'], when?: () => boolean): void {
  if (import.meta.env.DEV) {
    const slots = bound(handlers)
    const twice = slots.find((slot, i) => slots.indexOf(slot) !== i)
    if (twice) throw new Error(`keymap: ${twice} is bound twice in one registration`)
  }
  const registration = { handlers, when }
  if (registrations.push(registration) === 1) window.addEventListener('keydown', onKeydown)
  onScopeDispose(() => {
    registrations.splice(registrations.indexOf(registration), 1)
    if (registrations.length === 0) window.removeEventListener('keydown', onKeydown)
  })
}
