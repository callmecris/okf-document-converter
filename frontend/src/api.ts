export type Format = 'pdf' | 'docx' | 'epub'
export type JobStatus = 'pending' | 'processing' | 'completed' | 'failed'

export type BundleFile = {
  path: string
  /** Ruta relativa al API: /api/v1/jobs/{id}/bundle/{ruta} */
  url: string
}

export type Bundle = {
  path: string
  files: BundleFile[]
}

export type Job = {
  id: string
  user_id: string
  original_name: string
  format: Format
  object_key: string
  status: JobStatus
  error_message?: string
  created_at: string
  updated_at: string
  bundle?: Bundle
}

export type User = {
  id: string
  email: string
  created_at: string
}

const TOKEN_KEY = 'okf_token'

export const MAX_UPLOAD_MB = 100

const STATUS_MESSAGES: Record<number, string> = {
  400: 'Solicitud inválida. Verifica que el archivo sea uno de los formatos permitidos.',
  401: 'Tu sesión expiró. Inicia sesión de nuevo.',
  403: 'No tienes permiso para esta operación.',
  404: 'El documento solicitado no existe.',
  409: 'Ya existe una cuenta con ese correo.',
  413: `El archivo supera el tamaño máximo permitido (${MAX_UPLOAD_MB} MB).`,
  422: 'El archivo no cumple los requisitos del sistema.',
  429: 'Demasiadas solicitudes. Espera unos segundos e intenta de nuevo.',
  500: 'Error interno del servidor. Intenta de nuevo en unos minutos.',
  503: 'El servicio no está disponible. Intenta de nuevo en unos minutos.',
}

/** Traduce un HTTP status a un mensaje de error legible. */
export function friendlyError(status: number, apiMessage?: string): string {
  if (apiMessage) return apiMessage
  return STATUS_MESSAGES[status] ?? `Error inesperado (${status}). Intenta de nuevo.`
}

export function getToken(): string {
  return localStorage.getItem(TOKEN_KEY) ?? ''
}

export function setToken(token: string): void {
  localStorage.setItem(TOKEN_KEY, token)
}

export function clearToken(): void {
  localStorage.removeItem(TOKEN_KEY)
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers)
  const token = getToken()
  if (token) headers.set('Authorization', `Bearer ${token}`)

  const res = await fetch(`/api/v1${path}`, { ...init, headers })
  if (res.status === 401) {
    clearToken()
    window.dispatchEvent(new Event('okf:unauthorized'))
  }
  if (!res.ok) {
    let apiMessage: string | undefined
    try {
      const body = (await res.json()) as { error?: string }
      if (body.error) apiMessage = body.error
    } catch {
      /* body no JSON */
    }
    throw new Error(friendlyError(res.status, apiMessage))
  }
  return res.json() as Promise<T>
}

export function login(email: string, password: string): Promise<{ token: string; user: User }> {
  return request('/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, password }),
  })
}

export function register(email: string, password: string): Promise<{ token: string; user: User }> {
  return request('/auth/register', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, password }),
  })
}

export function listJobs(): Promise<Job[]> {
  return request('/jobs')
}

export function getJob(id: string): Promise<Job> {
  return request(`/jobs/${id}`)
}

export async function uploadJob(file: File): Promise<Job> {
  const form = new FormData()
  form.append('file', file)
  return request('/jobs', { method: 'POST', body: form })
}

/** Descarga el bundle completo (.zip) disparando la descarga del navegador. */
export async function downloadBundle(jobId: string, filename: string): Promise<void> {
  const res = await fetch(`/api/v1/jobs/${jobId}/download`, {
    headers: { Authorization: `Bearer ${getToken()}` },
  })
  if (!res.ok) throw new Error(friendlyError(res.status))
  const blob = await res.blob()
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.click()
  URL.revokeObjectURL(url)
}

/** Obtiene un archivo individual del bundle (autenticado) como blob. */
export async function fetchBundleFile(url: string): Promise<Blob> {
  const res = await fetch(url, {
    headers: { Authorization: `Bearer ${getToken()}` },
  })
  if (!res.ok) throw new Error(friendlyError(res.status))
  return res.blob()
}

/** Abre un archivo del bundle en una pestaña nueva (vía blob autenticado). */
export async function openBundleFile(url: string, fallbackName: string): Promise<void> {
  const blob = await fetchBundleFile(url)
  const objectURL = URL.createObjectURL(blob)
  const win = window.open('', '_blank')
  if (win) {
    win.document.title = fallbackName
    win.location.href = objectURL
  } else {
    const a = document.createElement('a')
    a.href = objectURL
    a.download = fallbackName
    a.click()
  }
  window.setTimeout(() => URL.revokeObjectURL(objectURL), 60_000)
}

export const FORMAT_LABEL: Record<Format, string> = {
  pdf: 'PDF',
  docx: 'DOCX',
  epub: 'EPUB',
}

export const STATUS_LABEL: Record<JobStatus, string> = {
  pending: 'Pendiente',
  processing: 'Procesando',
  completed: 'Completado',
  failed: 'Fallido',
}

export function formatDate(iso: string): string {
  return new Date(iso).toLocaleString('es', {
    day: '2-digit',
    month: 'short',
    hour: '2-digit',
    minute: '2-digit',
  })
}