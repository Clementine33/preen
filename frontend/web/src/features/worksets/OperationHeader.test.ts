import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import OperationHeader from './OperationHeader.vue'

const base = {
  title: '8', libraryName: '8', operation: null, counts: null, validationState: null, revisionLabel: null,
  generating: false, canGenerate: false, executeBlocked: null, showExecution: false, canceling: false, busy: false,
}

describe('OperationHeader', () => {
  it('does not repeat the title as the library name', () => {
    expect(mount(OperationHeader, { props: base }).findAll('p.font-mono')).toHaveLength(0)
    const other = mount(OperationHeader, { props: { ...base, libraryName: 'Lib' } })
    expect(other.get('p.font-mono').text()).toBe('Lib')
  })
})
