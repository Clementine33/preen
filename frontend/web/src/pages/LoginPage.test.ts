import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import { apiClientKey } from '@/lib/api/client'
import { apiStub } from '@/test/api-stub'
import LoginPage from './LoginPage.vue'

enableAutoUnmount(afterEach)
afterEach(() => vi.unstubAllGlobals())

async function mountLogin(createSession: (token: string) => Promise<void>, target = '/collections') {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/login', name: 'login', component: LoginPage },
    ],
  })
  await router.push({
    name: 'login',
    query: { redirect: target },
  })
  return mount(LoginPage, {
    global: {
      plugins: [router],
      provide: {
        [apiClientKey as symbol]: apiStub({ createSession }),
      },
    },
  })
}

describe('browser login', () => {
  it('submits the key then replaces the document at the normalized return address', async () => {
    const replace = vi.fn()
    vi.stubGlobal('location', { origin: 'http://localhost:3000', replace })
    const createSession = vi.fn().mockResolvedValue(undefined)
    const wrapper = await mountLogin(createSession, '/works/../collections')
    expect(wrapper.findAll('input')).toHaveLength(1)
    expect(wrapper.get('input').attributes('type')).toBe('password')
    await wrapper.get('input').setValue('shared;"\\secret')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(createSession).toHaveBeenCalledExactlyOnceWith('shared;"\\secret')
    expect(replace).toHaveBeenCalledExactlyOnceWith('/collections')
    expect((wrapper.get('input').element as HTMLInputElement).value).toBe('')
  })
  it('stays on login and clears a rejected key', async () => {
    const replace = vi.fn()
    vi.stubGlobal('location', { origin: 'http://localhost:3000', replace })
    const wrapper = await mountLogin(vi.fn().mockRejectedValue(new Error('rejected')))
    await wrapper.get('input').setValue('wrong')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('登录失败')
    expect((wrapper.get('input').element as HTMLInputElement).value).toBe('')
    expect(replace).not.toHaveBeenCalled()
  })
})
