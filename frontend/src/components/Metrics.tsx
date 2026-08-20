import { useCallback, useEffect, useState } from 'react'
import { getMetrics, STATUS_LABEL, type JobStatus, type Metrics as MetricsData } from '../api'

const STATUS_ORDER: JobStatus[] = ['pending', 'processing', 'completed', 'failed', 'canceled']

const STATUS_DOT: Record<JobStatus, string> = {
  pending: 'bg-amber-400',
  processing: 'bg-blue-400',
  completed: 'bg-emerald-400',
  failed: 'bg-rose-400',
  canceled: 'bg-slate-400',
}

type Props = {
  /** Cambia cuando la lista de trabajos se refresca, para recargar métricas. */
  refreshKey: number
}

/** Observabilidad básica del flujo de trabajos de toda la plataforma. */
export default function Metrics({ refreshKey }: Props) {
  const [data, setData] = useState<MetricsData | null>(null)
  const [open, setOpen] = useState(false)

  const load = useCallback(async () => {
    try {
      setData(await getMetrics())
    } catch {
      /* las métricas son informativas: un fallo no debe romper la vista */
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load, refreshKey])

  if (!data) return null

  const duration =
    data.avg_duration_seconds < 1
      ? `${Math.round(data.avg_duration_seconds * 1000)} ms`
      : `${data.avg_duration_seconds.toFixed(1)} s`

  return (
    <section className="rounded-2xl border border-slate-200 bg-white p-4 shadow-sm">
      <button
        onClick={() => setOpen(!open)}
        className="flex w-full items-center justify-between text-left"
      >
        <h2 className="text-sm font-bold uppercase tracking-wide text-slate-500">Métricas</h2>
        <span className="text-xs font-semibold text-indigo-600">
          {open ? 'Ocultar' : 'Ver detalle'}
        </span>
      </button>

      <div className="mt-3 flex flex-wrap gap-x-6 gap-y-2 text-sm">
        {STATUS_ORDER.filter((s) => data.jobs_by_status[s]).map((s) => (
          <span key={s} className="flex items-center gap-1.5 text-slate-600">
            <span className={`h-2 w-2 rounded-full ${STATUS_DOT[s]}`} />
            {STATUS_LABEL[s]}: <strong className="text-slate-800">{data.jobs_by_status[s]}</strong>
          </span>
        ))}
      </div>

      {open && (
        <dl className="mt-4 grid grid-cols-2 gap-3 border-t border-slate-100 pt-4 text-sm sm:grid-cols-3">
          <Stat label="Trabajos" value={data.total_jobs} />
          <Stat label="Bundles publicados" value={data.total_bundles} />
          <Stat label="Duración media" value={duration} />
          <Stat label="Reintentos" value={data.retries} />
          <Stat label="Usuarios" value={data.total_users} />
          <Stat
            label="Con advertencias"
            value={data.bundles_by_validation['valid_with_warnings'] ?? 0}
          />

          <div className="col-span-2 sm:col-span-3">
            <dt className="text-xs uppercase tracking-wide text-slate-400">Por formato</dt>
            <dd className="mt-1 flex flex-wrap gap-x-4 gap-y-1 text-slate-700">
              {Object.entries(data.jobs_by_format).map(([f, n]) => (
                <span key={f}>
                  {f.toUpperCase()}: <strong>{n}</strong>
                </span>
              ))}
            </dd>
          </div>

          <p className="col-span-2 text-xs text-slate-400 sm:col-span-3">
            También disponibles en formato Prometheus en <code>/metrics</code>.
          </p>
        </dl>
      )}
    </section>
  )
}

function Stat({ label, value }: { label: string; value: string | number }) {
  return (
    <div>
      <dt className="text-xs uppercase tracking-wide text-slate-400">{label}</dt>
      <dd className="text-lg font-bold text-slate-800">{value}</dd>
    </div>
  )
}
