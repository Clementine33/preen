import { expect, it } from 'vitest'
import { sessionFailureText } from './session-failure'

it('explains filesystem prechecks without claiming the disk is untouched', () => {
  for (const code of ['FILESYSTEM_OPERATION_UNSUPPORTED', 'FILESYSTEM_PRECHECK_FAILED']) {
    const message = sessionFailureText(code, 'scratch cleanup failed')
    expect(message).toContain('未开始转换')
    expect(message).not.toContain('未做任何改动')
  }
})
