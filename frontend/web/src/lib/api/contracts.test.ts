import { expect, it, vi } from 'vitest'
import { httpContracts, sseContracts, taskContract, taskEventsContract, taskSchema } from './contracts'

vi.mock('./contracts.gen', async (original) => {
  const actual = await original<typeof import('./contracts.gen')>()
  return {
    ...actual,
    taskSchemas: {
      ...actual.taskSchemas,
      partial: {
        1: { taskDraftEnvelope: 'partial_v1_taskDraftEnvelope' },
      },
    },
  }
})

it('maps generic schema names to the Task-specific schema', () => {
  expect(taskSchema('operationResponse', 'metadata_refresh')).toBe('metadata_refresh_v1_operationResponse')
  expect(taskSchema('draftDocumentRequest', 'conversion')).toBe('draftDocumentRequest')
  expect(taskSchema('draftDocumentRequest', 'rename')).toBe('dirrename_Draft_request')
  expect(taskContract(httpContracts['PUT /api/v1/worksets/{id}/operations/{type}/draft'], 'metadata_refresh').request).toBe('refresh_Draft_request')
  expect(taskEventsContract(sseContracts.execution, 'rename').component).toBe('rename_v1_executionComponentResponse')
})

it('passes through names shared by every Task', () => {
  expect(taskSchema('errorResponse', 'rename')).toBe('errorResponse')
})

it('throws on an unknown Task kind instead of using the generic schema', () => {
  expect(() => taskSchema('operationResponse', 'unknown')).toThrow('unknown Task kind unknown')
  expect(() => taskSchema('operationResponse', 'toString')).toThrow('unknown Task kind')
  expect(() => taskContract(httpContracts['PUT /api/v1/worksets/{id}/operations/{type}/draft'], 'unknown')).toThrow('unknown Task kind')
})

it('throws when a Task lacks an entry for a name other Tasks bind', () => {
  expect(taskSchema('taskDraftEnvelope', 'partial')).toBe('partial_v1_taskDraftEnvelope')
  expect(() => taskSchema('operationResponse', 'partial')).toThrow('Task partial has no schema bound to operationResponse')
})
