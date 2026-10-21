import { enableAutoUnmount, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import { usePlayerStore } from '@/stores/player'
import { mountPlayerKeys } from '@/test/player-keys'
import { useKeymap, type KeyId } from './use-keymap'

enableAutoUnmount(afterEach)

/** Mounts a component registering the given handlers, the way a page does. */
function register(handlers: Parameters<typeof useKeymap>[0], when?: () => boolean) {
  return mount(defineComponent({ setup: () => (useKeymap(handlers, when), () => h('div')) }))
}
function press(key: string, init: KeyboardEventInit = {}, target: EventTarget = window): KeyboardEvent {
  const event = new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true, ...init })
  target.dispatchEvent(event)
  return event
}

describe('keymap dispatcher', () => {
  it('installs one window listener while anything is registered', () => {
    const add = vi.spyOn(window, 'addEventListener')
    const remove = vi.spyOn(window, 'removeEventListener')
    const a = register({ 'wall.play': () => {} })
    const b = register({ 'detail.flip': () => {} })
    expect(add.mock.calls.filter(([type]) => type === 'keydown')).toHaveLength(1)
    a.unmount()
    expect(remove).not.toHaveBeenCalled()
    b.unmount()
    expect(remove.mock.calls.filter(([type]) => type === 'keydown')).toHaveLength(1)
    vi.restoreAllMocks()
  })

  it('keeps a text field, a menu and a dialog\'s keys, and leaves Ctrl/Meta/Alt to the browser', () => {
    const run = vi.fn()
    register({ 'wall.play': run })
    const input = document.body.appendChild(document.createElement('input'))
    const menu = document.body.appendChild(document.createElement('div'))
    menu.dataset.ownKeys = ''
    const dialog = document.body.appendChild(document.createElement('div'))
    dialog.setAttribute('role', 'dialog')
    for (const target of [input, menu, dialog]) press('p', {}, target)
    for (const mod of ['ctrlKey', 'metaKey', 'altKey']) press('p', { [mod]: true })
    expect(run).not.toHaveBeenCalled()
    expect(press('p').defaultPrevented).toBe(true)
    expect(run).toHaveBeenCalledTimes(1)
    for (const el of [input, menu, dialog]) el.remove()
  })

  it('leaves Enter and Space on a button or link to their native meaning, but not other keys', () => {
    const enter = vi.fn()
    const play = vi.fn()
    register({ 'wall.open': enter, 'wall.play': play })
    const button = document.body.appendChild(document.createElement('button'))
    press('Enter', {}, button)
    expect(enter).not.toHaveBeenCalled()
    press('p', {}, button)
    expect(play).toHaveBeenCalledTimes(1)
    button.remove()
  })

  it('matches modifiers exactly: Shift+ArrowRight is not ArrowRight', () => {
    const step = vi.fn()
    register({ 'detail.step': step })
    press('ArrowRight', { shiftKey: true })
    expect(step).not.toHaveBeenCalled()
    press('ArrowRight')
    expect(step).toHaveBeenCalledTimes(1)
  })

  it('checks the page layer before the player layer, first match wins, and a declined key falls through', () => {
    setActivePinia(createPinia())
    const page = vi.fn()
    const decline = vi.fn(() => false as const)
    const toggle = vi.fn()
    register({ 'player.toggle': toggle })
    register({ 'wall.open': decline })
    // Player keys are checked after every page key, whatever the order of registration.
    const second = register({ 'detail.close': page })
    press('Escape')
    expect(page).toHaveBeenCalledTimes(1)
    press('Enter')
    expect(decline).toHaveBeenCalledTimes(1)
    second.unmount()
    press(' ')
    expect(toggle).toHaveBeenCalledTimes(1)
  })

  it('suspends the page layer, not the player layer, while a modal dialog is open', () => {
    const page = vi.fn()
    const player = vi.fn()
    register({ 'wall.play': page })
    register({ 'player.mute': player })
    const dialog = document.body.appendChild(document.createElement('dialog'))
    dialog.setAttribute('open', '')
    press('p')
    press('m')
    expect(page).not.toHaveBeenCalled()
    expect(player).toHaveBeenCalledTimes(1)
    dialog.remove()
    press('p')
    expect(page).toHaveBeenCalledTimes(1)
  })

  it('pauses a registration while its condition is false', () => {
    const run = vi.fn()
    let on = false
    register({ 'wall.play': run }, () => on)
    press('p')
    on = true
    press('p')
    expect(run).toHaveBeenCalledTimes(1)
  })

  it('throws in development when a layer binds one key twice', () => {
    // wall.move and detail.cursor share the arrows, and both are page keys.
    const handlers: Partial<Record<KeyId, () => void>> = { 'wall.move': () => {}, 'detail.cursor': () => {} }
    vi.spyOn(console, 'warn').mockImplementation(() => {})
    expect(() => register(handlers)).toThrow(/ArrowUp/)
    vi.restoreAllMocks()
  })
})

describe('player keys', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })
  const load = () => {
    const player = usePlayerStore()
    player.play('RJ01000001', [{ title: 'a' }, { title: 'b' }, { title: 'c' }])
    return player
  }

  it('do nothing until something is loaded', () => {
    mountPlayerKeys()
    const player = usePlayerStore()
    expect(press(' ').defaultPrevented).toBe(false)
    expect(player.paused).toBe(true)
  })

  it('toggle with Space, mute with M, step with comma and period', () => {
    mountPlayerKeys()
    const player = load()
    press(' ')
    expect(player.paused).toBe(true)
    press('m')
    expect(player.muted).toBe(true)
    press('.')
    expect(player.trackIndex).toBe(1)
    press(',')
    expect(player.trackIndex).toBe(0)
    press(',')
    expect(player.trackIndex).toBe(0) // past the start nothing changes
  })

  it('seek 10 seconds with Shift+arrows, within the track', () => {
    mountPlayerKeys()
    const player = load()
    player.duration = 25
    press('ArrowRight', { shiftKey: true })
    expect(player.elapsed).toBe(10)
    press('ArrowRight', { shiftKey: true })
    press('ArrowRight', { shiftKey: true })
    expect(player.elapsed).toBe(25)
    press('ArrowLeft', { shiftKey: true })
    expect(player.elapsed).toBe(15)
  })

  it('change the volume 10% with Shift+up/down, unmuting on the way up', () => {
    mountPlayerKeys()
    const player = load()
    player.setVolume(0.5)
    press('ArrowUp', { shiftKey: true })
    expect(player.volume).toBe(0.6)
    press('ArrowDown', { shiftKey: true })
    press('ArrowDown', { shiftKey: true })
    expect(player.volume).toBe(0.4)
    player.muted = true
    press('ArrowUp', { shiftKey: true })
    expect(player.muted).toBe(false)
  })

  it('work on a page whose own keys use the same arrows', () => {
    mountPlayerKeys()
    const player = load()
    const step = vi.fn()
    register({ 'detail.step': step })
    press('ArrowRight')
    press('ArrowRight', { shiftKey: true })
    expect(step).toHaveBeenCalledTimes(1)
    expect(player.elapsed).toBe(10)
  })
})

describe('media session', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    Reflect.deleteProperty(navigator, 'mediaSession')
  })

  it('mirrors the track and answers the system handlers', () => {
    setActivePinia(createPinia())
    const handlers: Record<string, (details: MediaSessionActionDetails) => void> = {}
    const session = { metadata: null as unknown, playbackState: 'none', setActionHandler: (action: string, fn: never) => (handlers[action] = fn) }
    Object.defineProperty(navigator, 'mediaSession', { value: session, configurable: true })
    vi.stubGlobal('MediaMetadata', class { init: object; constructor(init: object) { this.init = init } })
    mountPlayerKeys()
    const player = usePlayerStore()
    expect(session.metadata).toBeNull()

    player.play('RJ01000001', [{ title: 'a' }, { title: 'b' }])
    return Promise.resolve().then(() => {
      expect(session.metadata).toMatchObject({ init: { title: 'a', album: 'RJ01000001' } })
      expect(session.playbackState).toBe('playing')
      handlers.pause({ action: 'pause' })
      expect(player.paused).toBe(true)
      handlers.play({ action: 'play' })
      expect(player.paused).toBe(false)
      handlers.nexttrack({ action: 'nexttrack' })
      expect(player.trackIndex).toBe(1)
      handlers.previoustrack({ action: 'previoustrack' })
      expect(player.trackIndex).toBe(0)
      handlers.seekforward({ action: 'seekforward' })
      expect(player.elapsed).toBe(10)
      handlers.seekbackward({ action: 'seekbackward', seekOffset: 4 })
      expect(player.elapsed).toBe(6)
      handlers.seekto({ action: 'seekto', seekTime: 42 })
      expect(player.elapsed).toBe(42)
    })
  })
})
