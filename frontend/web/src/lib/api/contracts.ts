import type { ViewOf } from './contract-views'
import type { SchemaId } from './contracts.gen'
import type { WireSchemas } from './wire.gen'
import { httpContracts, sseContracts, taskSchemas } from './contracts.gen'

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
  return {
    ...contract,
    request: taskSchema(contract.request, kind),
    responses: Object.fromEntries(Object.entries(contract.responses).map(([status, name]) => [
      status,
      taskSchema(name, kind),
    ])),
  } as C
}

/** Maps a generic schema name to the Task-specific schema. */
export function taskSchema<N extends string>(name: N, kind: string): N {
  const bindings: Record<string, string> | undefined = Object.hasOwn(taskSchemas, kind)
    ? taskSchemas[kind as keyof typeof taskSchemas]
    : undefined
  if (!bindings)
    throw new Error(`unknown Task kind ${kind}`)
  if (Object.hasOwn(bindings, name))
    return bindings[name] as N
  // Any other name (an error envelope, `@empty`) is shared by every Task.
  if (Object.values(taskSchemas).some(bound => Object.hasOwn(bound, name)))
    throw new Error(`Task ${kind} has no schema bound to ${name}`)
  return name
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
