import { useTaskResultNotice } from './use-task-toast'
import type { Library, ScanMode } from '@/lib/api/types'
import { useQueryClient, type QueryClient } from '@tanstack/vue-query'
import { useApiClient } from '@/lib/api/client'
import { refreshOrRemoveQueries } from '@/queries/cache-sync'
import { queryKeys } from '@/queries/query-keys'
import { sweepWorksetCaches } from '@/queries/worksets'
import { sweepWorkCaches } from '@/queries/works'
import { useScanStore, type ScanStatus, type ScanTerminal, type ScanOutcome } from '@/stores/scan'

async function refreshLibraries(queryClient: QueryClient): Promise<void> {
  await queryClient.invalidateQueries({ queryKey: queryKeys.libraries.list() })
}

async function refreshLibraryDerived(queryClient: QueryClient, libraryId: string): Promise<void> {
  // Folders and trees are independent key families — refetching them
  // serially would add one full request round-trip per prefix after every
  // scan terminal. The pre-migration store parallelized the same refresh.
  await Promise.all([
    refreshOrRemoveQueries(queryClient, queryKeys.libraries.dirsPrefix(libraryId)),
    refreshOrRemoveQueries(queryClient, queryKeys.libraries.memberTreesPrefix(libraryId)),
  ])
}

// Maps the terminal scan outcome to the exact cache synchronization it
// requires. Plan lists are deliberately never touched: plan membership no
// longer depends on materialized folders.
//
// Materialized folder rows are replaced in one transaction only when the scan
// completes, so any non-completed outcome — user cancel (abort or SSE),
// confirmed error, or a transport failure before the first event — leaves
// folders/trees unchanged and only library metadata needs refreshing. The one
// conservative case is a transport terminal after events: the backend may
// have committed before the stream died.
export async function syncAfterScan(
  queryClient: QueryClient,
  libraryId: string,
  status: ScanStatus,
  terminal: ScanTerminal | null,
  streamStarted = true,
): Promise<void> {
  // A cancelled scan never materializes (the backend replaces folders in one
  // transaction only on completion), so derived caches stay valid.
  if (status === 'cancelled') {
    await refreshLibraries(queryClient)
    return
  }
  // Completed scan, or a transport failure after events (may have committed):
  // refresh library metadata and the library's derived caches (active
  // folders/trees refetch, inactive entries are dropped). A committed scan
  // also changes workset validation (stale is derived from the live
  // inventory), so the workset domain is conservatively swept.
  if (status === 'completed' || (terminal !== 'event' && streamStarted)) {
    await Promise.all([
      refreshLibraries(queryClient),
      refreshLibraryDerived(queryClient, libraryId),
      sweepWorksetCaches(queryClient),
      sweepWorkCaches(queryClient),
    ])
    return
  }
  // Confirmed cancel/error over SSE, or a transport terminal before any event:
  // nothing was committed — only scan metadata changed.
  await refreshLibraries(queryClient)
}

// Single entry point for scan orchestration: pages call start/cancel and
// never assemble invalidation themselves. The scan SSE lifecycle stays in the
// scan Pinia store; this module owns cache coordination and result notices.
// The awaited request survives page unmount; ownership is read from the live router.
export function useLibraryScan() {
  const api = useApiClient()
  const queryClient = useQueryClient()
  const scan = useScanStore()
  const notice = useTaskResultNotice(queryClient)

  async function start(libraryId: string, mode?: ScanMode): Promise<void> {
    const libraryName = queryClient.getQueryData<Library[]>(queryKeys.libraries.list())
      ?.find((library) => library.id === libraryId)?.name ?? libraryId
    const outcome = await scan.start(libraryId, api, notify, mode)
    // Refused or superseded requests have no result of their own.
    if (!outcome) return
    await syncAfterScan(queryClient, libraryId, outcome.status, outcome.terminal, outcome.receivedEvent)

    function notify({ status, terminal, admissionError }: ScanOutcome) {
      if (admissionError) return
      const localCancel = terminal === 'transport' && status === 'cancelled'
      const unknown = terminal === 'transport' && !localCancel
      const result = localCancel ? '扫描停止已请求，结果尚未确认'
        : unknown ? '连接中断，扫描结果尚未确认'
          : status === 'completed' ? '扫描完成'
            : status === 'cancelled' ? '扫描已停止' : '扫描失败'
      const tone = localCancel || unknown ? 'unknown' : status === 'completed' ? 'success' : status === 'cancelled' ? 'stopped' : 'failed'
      notice({
        libraryId, libraryName, title: `${libraryName} · 扫描`, text: result, tone, target: 'workbench-overview',
        // Its own overview shows a settled result, including a cancel still being confirmed.
        quietOn: status === 'completed' || status === 'cancelled' ? ['workbench-overview', 'overview-files'] : [],
      })
    }
  }

  function cancel(): void {
    scan.cancel()
  }

  return { start, cancel }
}
