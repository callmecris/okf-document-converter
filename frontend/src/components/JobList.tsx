import { useState } from 'react'
import {
  cancelJob,
  downloadBundle,
  formatDate,
  FORMAT_LABEL,
  getJob,
  openBundleFile,
  retryJob,
  STATUS_LABEL,
  type BundleFile,
  type Job,
} from '../api'

const STATUS_STYLE: Record<Job['status'], string> = {
  pending: 'bg-amber-100 text-amber-700',
  processing: 'bg-blue-100 text-blue-700 animate-pulse',
  completed: 'bg-emerald-100 text-emerald-700',
  failed: 'bg-rose-100 text-rose-700',
  canceled: 'bg-slate-200 text-slate-600',
}

const ICON_COLOR: Record<Job['format'], string> = {
  md: 'bg-slate-100 text-slate-600',
  txt: 'bg-stone-100 text-stone-600',
  html: 'bg-orange-100 text-orange-600',
  pdf: 'bg-rose-100 text-rose-600',
  docx: 'bg-sky-100 text-sky-600',
  epub: 'bg-violet-100 text-violet-600',
}

type Props = {
  jobs: Job[]
  /** Se invoca tras reintentar, para refrescar la lista. */
  onRetried?: () => void
}

export default function JobList({ jobs, onRetried }: Props) {
  const [expanded, setExpanded] = useState<string | null>(null)
  const [zipBusy, setZipBusy] = useState<string | null>(null)
  const [zipError, setZipError] = useState('')
  const [retryBusy, setRetryBusy] = useState<string | null>(null)
  // GET /jobs no incluye el bundle (solo GET /jobs/{id}): se pide bajo demanda
  // para poder mostrar los archivos y las advertencias de validación.
  const [details, setDetails] = useState<Record<string, Job>>({})
  const [cancelBusy, setCancelBusy] = useState<string | null>(null)

  const cancel = async (job: Job) => {
    setCancelBusy(job.id)
    setZipError('')
    try {
      await cancelJob(job.id)
      onRetried?.()
    } catch (err) {
      setZipError(err instanceof Error ? err.message : 'Error al cancelar')
    } finally {
      setCancelBusy(null)
    }
  }

  const retry = async (job: Job) => {
    setRetryBusy(job.id)
    setZipError('')
    try {
      await retryJob(job.id)
      onRetried?.()
    } catch (err) {
      setZipError(err instanceof Error ? err.message : 'Error al reintentar')
    } finally {
      setRetryBusy(null)
    }
  }

  const toggle = async (id: string) => {
    const next = expanded === id ? null : id
    setExpanded(next)
    setZipError('')
    if (next && !details[next]) {
      try {
        const full = await getJob(next)
        setDetails((prev) => ({ ...prev, [next]: full }))
      } catch (err) {
        setZipError(err instanceof Error ? err.message : 'Error al cargar el bundle')
      }
    }
  }

  const zip = async (job: Job) => {
    setZipBusy(job.id)
    setZipError('')
    try {
      const base = job.original_name.replace(/\.(md|txt|html?|pdf|docx|epub)$/i, '')
      await downloadBundle(job.id, `${base}-okf.zip`)
    } catch (err) {
      setZipError(err instanceof Error ? err.message : 'Error al descargar')
    } finally {
      setZipBusy(null)
    }
  }

  if (jobs.length === 0) {
    return (
      <div className="rounded-2xl border border-dashed border-slate-300 bg-white/60 p-10 text-center">
        <p className="text-sm text-slate-500">
          No tienes conversiones todavía. Sube tu primer documento arriba.
        </p>
      </div>
    )
  }

  const sorted = [...jobs].sort(
    (a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime(),
  )

  return (
    <div className="space-y-4">
      {zipError && (
        <div className="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-600">
          {zipError}
        </div>
      )}

      {sorted.map((job) => {
        const open = expanded === job.id
        const done = job.status === 'completed'
        const detail = details[job.id]
        return (
          <div
            key={job.id}
            className="overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-sm transition hover:shadow-md"
          >
            <div className="flex items-center gap-4 p-4">
              <div
                className={`flex h-10 w-10 shrink-0 items-center justify-center rounded-xl text-xs font-bold uppercase ${ICON_COLOR[job.format]}`}
              >
                {FORMAT_LABEL[job.format]}
              </div>

              <div className="min-w-0 flex-1">
                <p className="truncate font-semibold text-slate-800">{job.original_name}</p>
                <p className="text-xs text-slate-400">{formatDate(job.created_at)}</p>
              </div>

              <span
                className={`shrink-0 rounded-full px-3 py-1 text-xs font-semibold ${STATUS_STYLE[job.status]}`}
              >
                {STATUS_LABEL[job.status]}
              </span>

              {(job.status === 'pending' || job.status === 'processing') && (
                <button
                  onClick={() => void cancel(job)}
                  disabled={cancelBusy === job.id}
                  className="shrink-0 rounded-lg border border-slate-300 px-3 py-1.5 text-xs font-semibold text-slate-600 transition hover:bg-slate-50 disabled:opacity-50"
                >
                  {cancelBusy === job.id ? 'Cancelando…' : 'Cancelar'}
                </button>
              )}

              {done && (
                <>
                  <button
                    onClick={() => void zip(job)}
                    disabled={zipBusy === job.id}
                    className="shrink-0 rounded-lg bg-indigo-600 px-3 py-1.5 text-xs font-semibold text-white transition hover:bg-indigo-700 disabled:opacity-50"
                  >
                    {zipBusy === job.id ? 'Preparando…' : 'Descargar bundle (.zip)'}
                  </button>
                  <button
                    onClick={() => void toggle(job.id)}
                    className="shrink-0 rounded-lg border border-slate-300 px-3 py-1.5 text-xs font-semibold text-slate-600 transition hover:bg-slate-50"
                  >
                    {open ? 'Ocultar' : 'Archivos'}
                  </button>
                </>
              )}
            </div>

            {job.status === 'canceled' && (
              <div className="flex items-center gap-3 border-t border-slate-100 bg-slate-50 px-4 py-2 text-xs text-slate-500">
                <span className="min-w-0 flex-1">
                  Cancelado por el usuario. No se publicó ningún bundle.
                </span>
                <button
                  onClick={() => void retry(job)}
                  disabled={retryBusy === job.id}
                  className="shrink-0 rounded-lg border border-slate-300 px-3 py-1 font-semibold text-slate-600 transition hover:bg-slate-100 disabled:opacity-50"
                >
                  {retryBusy === job.id ? 'Reencolando…' : 'Reintentar'}
                </button>
              </div>
            )}

            {job.status === 'failed' && (
              <div className="flex items-center gap-3 border-t border-slate-100 bg-rose-50/50 px-4 py-2 text-xs text-rose-600">
                <span className="min-w-0 flex-1 truncate" title={job.error_message}>
                  {job.error_message || 'La conversión falló.'}
                </span>
                <button
                  onClick={() => void retry(job)}
                  disabled={retryBusy === job.id}
                  className="shrink-0 rounded-lg border border-rose-300 px-3 py-1 font-semibold text-rose-700 transition hover:bg-rose-100 disabled:opacity-50"
                >
                  {retryBusy === job.id ? 'Reencolando…' : 'Reintentar'}
                </button>
              </div>
            )}

            {job.retry_of && (
              <div className="border-t border-slate-100 bg-slate-50 px-4 py-1.5 text-xs text-slate-500">
                Reintento del trabajo <code className="text-slate-600">{job.retry_of.slice(0, 8)}</code>{' '}
                · intento {job.attempt}
              </div>
            )}

            {open && done && detail?.bundle?.validation === 'valid_with_warnings' && (
              <div className="border-t border-slate-100 bg-amber-50 px-4 py-2 text-xs text-amber-800">
                <p className="font-semibold">Válido con advertencias</p>
                <ul className="mt-1 list-disc space-y-0.5 pl-4">
                  {detail.bundle.warnings.map((w) => (
                    <li key={w}>{w}</li>
                  ))}
                </ul>
              </div>
            )}

            {open && done && !detail && (
              <div className="border-t border-slate-100 bg-slate-50/70 px-4 py-3 text-xs text-slate-400">
                Cargando archivos del bundle…
              </div>
            )}

            {open && done && detail?.bundle && (
              <div className="space-y-3 border-t border-slate-100 bg-slate-50/70 p-4">
                <FileGroup
                  title="Bundle"
                  files={detail.bundle.files.filter((f) => !f.path.includes('/assets/'))}
                />
                <FileGroup
                  title="Recursos (assets)"
                  files={detail.bundle.files.filter((f) => f.path.includes('/assets/'))}
                />
              </div>
            )}
          </div>
        )
      })}
    </div>
  )
}

/** Lista un grupo de archivos del bundle; no renderiza nada si está vacío. */
function FileGroup({ title, files }: { title: string; files: BundleFile[] }) {
  if (files.length === 0) return null
  return (
    <div>
      <p className="mb-1 text-xs font-semibold uppercase tracking-wide text-slate-400">{title}</p>
      <ul className="space-y-1">
        {files.map((f) => {
          const name = f.path.split('/').pop() ?? f.path
          return (
            <li key={f.path} className="flex items-center gap-2 text-sm">
              <span className="text-slate-400">·</span>
              <button
                onClick={() => void openBundleFile(f.url, name)}
                className="truncate text-indigo-600 underline-offset-2 hover:underline"
                title={f.path}
              >
                {name}
              </button>
            </li>
          )
        })}
      </ul>
    </div>
  )
}
