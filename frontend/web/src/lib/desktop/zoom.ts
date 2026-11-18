/** The desktop main window's UI zoom: kept in localStorage (per machine), applied by the Wails adapter at startup. */
export const ZOOM_STORAGE_KEY = 'preen-zoom'

export const ZOOM_OPTIONS: readonly { value: string, label: string }[] = [
  { value: '0.9', label: '90%' },
  { value: '1', label: '100%' },
  { value: '1.1', label: '110%' },
  { value: '1.25', label: '125%' },
  { value: '1.5', label: '150%' },
]

/** The stored factor; a missing, invalid or unreadable value is 100%. */
export function readZoom(): number {
  try {
    const stored = localStorage.getItem(ZOOM_STORAGE_KEY)
    return ZOOM_OPTIONS.some(option => option.value === stored)
      ? Number(stored)
      : 1
  }
  catch {
    return 1
  }
}

export function storeZoom(factor: number): void {
  try {
    localStorage.setItem(ZOOM_STORAGE_KEY, String(factor))
  }
  catch {
    /* storage unavailable; the choice still applies this session */
  }
}
