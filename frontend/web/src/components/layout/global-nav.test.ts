import { describe, expect, it } from 'vitest'
import { router } from '@/app/router'
import { GLOBAL_NAV, globalNavOwner } from './global-nav'

describe('global navigation definition', () => {
  it('lists the work catalog and collections before the workbench', () => {
    expect(GLOBAL_NAV.map(item => ({ id: item.id, label: item.label, to: item.to }))).toEqual([
      { id: 'works', label: '作品库', to: '/works' },
      { id: 'collections', label: '合集', to: '/collections' },
      { id: 'worksets', label: '工作集', to: '/worksets' },
    ])
  })
})

describe('global navigation ownership', () => {
  // The mapping is explicit on purpose: asserting it against the real
  // route table means a renamed or added route fails here instead of silently
  // losing its highlight.
  it('assigns every route inside the application shell to a global entry', () => {
    const named = router.getRoutes().filter(record => typeof record.name === 'string' && record.name !== 'login')
    expect(named.length).toBeGreaterThan(0)
    for (const record of named) {
      expect(globalNavOwner(record.name), String(record.name)).not.toBeNull()
    }
  })

  it('leaves the login page outside global navigation', () => {
    expect(router.resolve('/login').name).toBe('login')
    expect(globalNavOwner('login')).toBeNull()
  })

  it('keeps the work catalog routes under 作品库', () => {
    for (const name of ['works', 'work-detail'] as const) {
      expect(globalNavOwner(name), name).toBe('works')
    }
  })

  it('keeps the collections page under 合集', () => {
    expect(globalNavOwner('collections')).toBe('collections')
  })

  it('keeps every workbench route under 工作集', () => {
    for (const name of [
      'worksets',
      'workbench-overview',
      'overview-files',
      'conversion',
      'conversion-member-files',
      'conversion-settings',
      'conversion-member',
      'conversion-member-edit',
      'conversion-batch-edit',
      'conversion-execution',
    ]) {
      expect(globalNavOwner(name), name).toBe('worksets')
    }
  })

  it('has no owner for unknown or missing route names', () => {
    expect(globalNavOwner(undefined)).toBeNull()
    expect(globalNavOwner(Symbol('anonymous'))).toBeNull()
    expect(globalNavOwner('not-a-route')).toBeNull()
  })
})
