import type { ViewOf } from './contract-views'
import type { SchemaId } from './contracts.gen'
import type { WireSchemas } from './wire.gen'
import { httpContracts, schemaIds, sseContracts } from './contracts.gen'

export { httpContracts, sseContracts }
export type { SchemaId }

export interface HttpContract {
  readonly request: SchemaId | ''
  readonly responses: Readonly<Record<string, SchemaId | '@empty' | '@sse'>>
}
export type SSEContract = Readonly<Record<string, SchemaId>>
type Values<T> = T[keyof T]
export type HttpResult<C extends HttpContract> = ViewOf<Extract<Values<C['responses']>, SchemaId>> | ('@empty' extends Values<C['responses']> ? void : never)
export type SSEViews<C extends SSEContract> = { [K in keyof C]: { type: K, data: ViewOf<C[K]> } }[keyof C]
export type WireBody<C extends HttpContract> = C['request'] extends SchemaId ? Pick<WireSchemas[C['request']], keyof WireSchemas[C['request']]> : unknown

export function taskContract<C extends HttpContract>(contract: C, kind: string): C {
  let request: string = taskSchema(contract.request, kind)
  if (contract.request === 'draftDocumentRequest' && kind !== 'conversion') {
    request = kind === 'rename'
      ? 'dirrename_Draft_request'
      : 'refresh_Draft_request'
  }
  return {
    ...contract,
    request,
    responses: Object.fromEntries(Object.entries(contract.responses).map(([status, name]) => [
      status,
      taskSchema(name, kind),
    ])),
  } as C
}
export function taskSchema<N extends string>(name: N, kind: string): N {
  const candidate = `${kind}_v1_${name}`
  return (schemaIds.includes(candidate as SchemaId)
    ? candidate
    : name) as N
}

export function executionContract(view?: 'members', memberId?: string) {
  const name = view === 'members'
    ? 'executionMembersViewResponse'
    : memberId ? 'executionMemberViewResponse' : 'executionFullViewResponse'
  return {
    request: '',
    responses: { 200: name },
  } as const
}
export function executionEventsContract(view?: 'members') {
  return {
    ...sseContracts.execution,
    execution_snapshot: view === 'members'
      ? 'executionMembersViewResponse'
      : 'executionFullViewResponse',
  } as const
}

export function taskEventsContract<C extends SSEContract>(contract: C, kind: string): C {
  return Object.fromEntries(Object.entries(contract).map(([event, name]) => [
    event,
    taskSchema(name, kind),
  ])) as C
}
