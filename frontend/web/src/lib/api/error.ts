import { ApiError } from './client'

export function errorDetails(error: unknown, fallback = '发生未知错误'): { code: string | null, message: string } {
  if (error instanceof ApiError)
    return { code: error.code, message: error.message }
  if (error instanceof Error && error.message)
    return { code: null, message: error.message }
  return { code: null, message: fallback }
}
