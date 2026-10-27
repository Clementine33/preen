import { router } from './router'

// Authentication switches replace the document so caches, media and event
// subscriptions restart together (ADR 0015).
let redirecting = false

export function redirectToLogin(): void {
  if (redirecting || router.resolve(location.pathname).name === 'login')
    return
  redirecting = true
  const target = location.pathname + location.search + location.hash
  location.replace(`/login?redirect=${encodeURIComponent(target)}`)
}

export function loginReturnAddress(value: unknown): string {
  if (typeof value !== 'string' || !value.startsWith('/') || value[1] === '/' || value[1] === '\\')
    return '/'
  try {
    const target = new URL(value, location.origin)
    if (target.origin !== location.origin || router.resolve(target.pathname).name === 'login')
      return '/'
    return target.pathname + target.search + target.hash
  }
  catch {
    return '/'
  }
}
