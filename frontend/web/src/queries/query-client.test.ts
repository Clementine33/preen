import { describe, expect, it } from 'vitest'
import { createAppQueryClient } from './query-client'

describe('createAppQueryClient', () => {
  it('applies the explicit-sync defaults', () => {
    const client = createAppQueryClient()
    const defaults = client.getDefaultOptions()

    expect(defaults.queries).toMatchObject({
      staleTime: Infinity,
      retry: false,
      refetchOnMount: false,
      refetchOnWindowFocus: false,
      refetchOnReconnect: false,
    })
    expect(defaults.mutations).toMatchObject({ retry: false })
  })
})
