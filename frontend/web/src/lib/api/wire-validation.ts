import type { HttpContract, SSEContract } from './contracts'
import type { SchemaId } from './contracts.gen'
import type { WireSchemas } from './wire.gen'

export class WireContractError extends Error {
  constructor(message: string) {
    super(`PREEN_WIRE_CONTRACT_VALIDATION: ${message}`)
    this.name = 'WireContractError'
  }
}

/** A schema ID selects its generated TypeScript type as well as its validator. */
export async function validateWire<K extends SchemaId>(name: K, data: unknown): Promise<WireSchemas[K]> {
  const validators = await import('./validators.gen.js')
  const validate = validators[name]
  if (typeof validate !== 'function')
    throw new WireContractError(`unknown schema ${name}`)
  if (!validate(data))
    throw new WireContractError(`${name}: ${JSON.stringify(validate.errors)}`)
  return data as WireSchemas[K]
}

export async function validateHTTP(contract: HttpContract | undefined, status: number, data: unknown): Promise<void> {
  if (!contract)
    throw new WireContractError('missing HTTP contract')
  const name = contract.responses[String(status)]
  if (!name)
    throw new WireContractError(`unmapped HTTP status ${status}`)
  if (name === '@empty') {
    if (data !== undefined)
      throw new WireContractError(`unexpected body for HTTP ${status}`)
    return
  }
  if (name === '@sse')
    throw new WireContractError('SSE response sent to JSON entry')
  await validateWire(name, data)
}

export async function validateEvent(contract: SSEContract, type: string, data: unknown): Promise<void> {
  const name = contract[type]
  if (!Object.hasOwn(contract, type) || !name)
    throw new WireContractError(`unmapped SSE event ${type}`)
  await validateWire(name, data)
}

export function validateSSEStatus(status: number): void {
  if (status !== 200)
    throw new WireContractError(`unmapped SSE HTTP status ${status}`)
}
