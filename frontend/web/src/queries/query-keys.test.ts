import { describe, expect, it } from 'vitest'
import { queryKeys } from './query-keys'

describe('queryKeys', () => {
  it('keeps libraries list shape stable', () => {
    expect(queryKeys.libraries.list()).toEqual(['libraries', 'list'])
  })

  it('isolates the overview listing by library', () => {
    expect(queryKeys.libraries.dirs('lib-a')).toEqual(['libraries', 'dirs', 'lib-a'])
    expect(queryKeys.libraries.dirs('lib-b')).not.toEqual(queryKeys.libraries.dirs('lib-a'))
  })

  it('isolates member trees by library and member path', () => {
    const key = queryKeys.libraries.memberTree('lib-a', 'albumA')
    expect(key).toEqual([
      'libraries',
      'member-trees',
      'lib-a',
      'albumA',
    ])
    expect(queryKeys.libraries.memberTree('lib-a', 'albumB')).not.toEqual(key)
    expect(queryKeys.libraries.memberTree('lib-b', 'albumA')).not.toEqual(key)
  })

  it('supports prefix invalidation over the trees of one library', () => {
    expect(queryKeys.libraries.memberTree('lib-a', 'albumA').slice(0, 3)).toEqual(
      queryKeys.libraries.memberTreesPrefix('lib-a'),
    )
  })

  it('addresses a library\'s current record by the library, shared across tasks', () => {
    expect(queryKeys.worksets.current('lib-a', 'conversion')).toEqual(['worksets', 'current', 'lib-a'])
    expect(queryKeys.worksets.current('lib-a', 'rename')).toEqual(queryKeys.worksets.current('lib-a', 'conversion'))
    expect(queryKeys.worksets.current('lib-b', 'conversion')).not.toEqual(
      queryKeys.worksets.current('lib-a', 'conversion'),
    )
  })

  it('isolates plan lists by library and limit', () => {
    expect(queryKeys.plans.list('lib-a', 100)).toEqual(['plans', 'list', 'lib-a', 100])
    expect(queryKeys.plans.list('lib-a', 50)).not.toEqual(queryKeys.plans.list('lib-a', 100))
    expect(queryKeys.plans.list('lib-b', 100)).not.toEqual(queryKeys.plans.list('lib-a', 100))
  })

  it('keeps plan detail keyed by plan ID', () => {
    expect(queryKeys.plans.detail('plan-1')).toEqual(['plans', 'detail', 'plan-1'])
    expect(queryKeys.plans.detail('plan-2')).not.toEqual(queryKeys.plans.detail('plan-1'))
  })
})
