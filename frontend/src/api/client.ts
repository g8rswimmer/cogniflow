import type { ApiErrorBody, FieldValidationError } from './types'

export const API_BASE = (import.meta.env.VITE_API_BASE as string | undefined) ?? ''

export class ApiError extends Error {
  status: number
  code: string
  validationErrors: FieldValidationError[]

  constructor(status: number, body: ApiErrorBody) {
    super(body.error?.message ?? `HTTP ${status}`)
    this.status = status
    this.code = body.error?.code ?? 'UNKNOWN'
    this.validationErrors = body.error?.details?.validation_errors ?? []
  }
}

export async function request<T>(path: string, init?: RequestInit): Promise<T> {
  // Inject Bearer token if present. Imported lazily to avoid circular dep.
  const { useAuthStore } = await import('../stores/useAuthStore')
  const token = useAuthStore.getState().token

  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(init?.headers as Record<string, string> ?? {}),
  }
  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }

  const res = await fetch(`${API_BASE}/v1${path}`, { ...init, headers })

  if (res.status === 401) {
    useAuthStore.getState().logout()
    window.location.href = '/login'
    throw new ApiError(401, { error: { code: 'UNAUTHORIZED', message: 'Session expired' } })
  }

  if (!res.ok) {
    const body: ApiErrorBody = await res.json().catch(() => ({}))
    throw new ApiError(res.status, body)
  }
  if (res.status === 204) return undefined as T
  return res.json() as Promise<T>
}

// publicRequest skips auth injection — used for login, invite preview, accept-invite.
export async function publicRequest<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${API_BASE}/v1${path}`, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...(init?.headers as Record<string, string> ?? {}),
    },
  })
  if (!res.ok) {
    const body: ApiErrorBody = await res.json().catch(() => ({}))
    throw new ApiError(res.status, body)
  }
  if (res.status === 204) return undefined as T
  return res.json() as Promise<T>
}
