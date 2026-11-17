import { describe, expect, it } from 'vitest'
import { queryKeys } from './query-keys'

describe('queryKeys', () => {
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
})
