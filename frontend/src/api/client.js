export class ApiError extends Error {
  constructor(message, { status = 0, code = 'request_failed', fields = {} } = {}) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.fields = fields
  }
}

export async function apiFetch(path, options = {}) {
  const headers = new Headers(options.headers)
  const isForm = options.body instanceof FormData
  if (options.body && !isForm && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }
  headers.set('Accept', 'application/json')

  let response
  try {
    response = await fetch(`/api/v1${path}`, {
      ...options,
      headers,
      credentials: 'include',
    })
  } catch {
    throw new ApiError('The server could not be reached. Check your connection.')
  }

  const contentType = response.headers.get('content-type') || ''
  const payload = contentType.includes('application/json') ? await response.json() : null
  if (!response.ok) {
    const detail = payload?.error
    throw new ApiError(detail?.message || `Request failed (${response.status}).`, {
      status: response.status,
      code: detail?.code,
      fields: detail?.fields,
    })
  }
  return payload?.data ?? payload
}

export const api = {
  me: () => apiFetch('/auth/me'),
  login: (credentials) => apiFetch('/auth/login', { method: 'POST', body: JSON.stringify(credentials) }),
  logout: () => apiFetch('/auth/logout', { method: 'POST' }),
  dashboard: () => apiFetch('/dashboard'),
  nodes: (params = {}) => {
    const query = new URLSearchParams(Object.entries(params).filter(([, value]) => value))
    return apiFetch(`/nodes${query.size ? `?${query}` : ''}`)
  },
  node: (id) => apiFetch(`/nodes/${id}`),
  backup: (id) => apiFetch(`/backups/${id}`),
  backupDiff: (id) => apiFetch(`/backups/${id}/diff`),
  settings: (tab) => apiFetch(`/${tab}`),
}
