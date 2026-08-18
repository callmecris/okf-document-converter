import { useState } from 'react'
import {
  downloadBundle,
  formatDate,
  FORMAT_LABEL,
  openBundleFile,
  STATUS_LABEL,
  type Job,
} from '../api'

const STATUS_STYLE: Record<Job['status'], string> = {
  pending: 'bg-amber-100 text-amber-700',
  processing: 'bg-blue-100 text-blue-700 animate-pulse',
  completed: 'bg-emerald-100 text-emerald-700',
  failed: 'bg-rose-100 text-rose-700',
}

const ICON_COLOR: Record<Job['format'], string> = {
  pdf: 'bg-rose-100 text-rose-600',
  docx: 'bg-sky-100 text-sky-600',
  epub: 'bg-violet-100 text-violet-600',
}

type Props = {
  jobs: Job[]
}

export default function JobList({ jobs }: Props) {
  const [expanded, setExpanded] = useState<string | null>(null)
  const [zipBusy, setZipBusy] = useState<string | null>(null)
  const [zipError, setZipError] = useState('')

  const toggle = (id: string) => {
    setExpanded(expanded === id ? null : id)
    setZipError('')
  }

  const zip = async (job: Job) => {
    setZipBusy(job.id)
    setZipError('')
    try {
      const base = job.original_name.replace(/\.(pdf|docx|epub)$/i, '')
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
                    onClick={() => toggle(job.id)}
                    className="shrink-0 rounded-lg border border-slate-300 px-3 py-1.5 text-xs font-semibold text-slate-600 transition hover:bg-slate-50"
                  >
                    {open ? 'Ocultar' : 'Archivos'}
                  </button>
                </>
              )}
            </div>

            {job.status === 'failed' && job.error_message && (
              <div className="border-t border-slate-100 bg-rose-50/50 px-4 py-2 text-xs text-rose-600">
                {job.error_message}
              </div>
            )}

            {open && done && job.bundle && (
              <div className="border-t border-slate-100 bg-slate-50/70 p-4">
                <ul className="space-y-1">
                  {job.bundle.files.map((f) => {
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
            )}
          </div>
        )
      })}
    </div>
  )
}