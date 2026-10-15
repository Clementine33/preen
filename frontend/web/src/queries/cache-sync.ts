import { type QueryClient, type QueryFilters } from '@tanstack/vue-query'

// Synchronizes a family of queries under one key prefix after a mutation or
// scan terminal event:
//   - active (mounted) members are invalidated and refetch immediately;
//   - inactive members are removed, so the next mount cold-fetches.
// This avoids the "stale inactive query + refetchOnMount: false" trap where
// an invalidated query would otherwise sit in the cache forever without ever
// refreshing.
export async function refreshOrRemoveQueries(
  queryClient: QueryClient,
  prefix: readonly unknown[],
  filters: Omit<QueryFilters, 'queryKey'> = {},
): Promise<void> {
  // An initial read may predate the commit; invalidation alone can keep that
  // request running when no data is cached yet. Cancel it before refetching.
  await queryClient.cancelQueries({ ...filters, queryKey: prefix })
  await queryClient.invalidateQueries({ ...filters, queryKey: prefix })
  const inactive = queryClient
    .getQueryCache()
    .findAll({ ...filters, queryKey: prefix })
    .filter((query) => query.getObserversCount() === 0)
  for (const query of inactive) {
    queryClient.removeQueries({ queryKey: query.queryKey })
  }
}