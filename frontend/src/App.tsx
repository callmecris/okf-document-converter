import { useCallback, useEffect, useRef, useState } from 'react'
import { clearToken, getToken, listJobs, setToken as persistToken, type Job, type JobStatus } from './api'
import Login from './components/Login'
import Upload from './components/Upload'
import JobList from './components/JobList'
import Metrics from './components/Metrics'

const ACTIVE: JobStatus[] = ['pending', 'processing']

function statusCount(jobs: Job[]) {
  return jobs.filter((j) => ACTIVE.includes(j.status)).length
}

export default function App() {
  const [token, setToken] = useState<string>(() => getToken())
  const [jobs, setJobs] = useState<Job[]>([])
  const [error, setError] = useState('')
  // Se incrementa en cada recarga de trabajos para refrescar las métricas.
  const [refreshKey, setRefreshKey] = useState(0)
  const timer = useRef<number | null>(null)
  // Trabajos activos, en una ref para que el intervalo no dependa de `jobs`.
  const activeRef = useRef(0)

  // Token inválido/expirado: volver al login automáticamente.
  useEffect(() => {
    const onUnauthorized = () => {
      clearToken()
      setJobs([])
      setToken('')
    }
    window.addEventListener('okf:unauthorized', onUnauthorized)
    return () => window.removeEventListener('okf:unauthorized', onUnauthorized)
  }, [])

  const load = useCallback(async () => {
    if (!token) return
    try {
      const fresh = await listJobs()
      setJobs(fresh)
      activeRef.current = statusCount(fresh)
      setRefreshKey((k) => k + 1)
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Error al cargar trabajos')
    }
  }, [token])

  // Polling suave: cada 4s mientras haya trabajos pendientes/procesando.
  //
  // El intervalo NO puede depender de `jobs`: load() actualiza el estado, lo
  // que volveria a ejecutar el efecto y a llamar a load() en bucle. El numero
  // de trabajos activos se consulta por referencia, que no dispara renders.
  useEffect(() => {
    if (!token) return
    void load()
    if (timer.current !== null) window.clearInterval(timer.current)
    timer.current = window.setInterval(() => {
      if (activeRef.current > 0) void load()
    }, 4000)
    return () => {
      if (timer.current !== null) window.clearInterval(timer.current)
    }
  }, [token, load])

  const logout = () => {
    clearToken()
    setToken('')
    setJobs([])
  }

  if (!token) {
    return (
      <Login
        onAuth={(t) => {
          persistToken(t)
          setToken(t)
        }}
      />
    )
  }

  const active = statusCount(jobs)

  return (
    <div className="min-h-screen bg-slate-100">
      <header className="sticky top-0 z-10 border-b border-slate-200 bg-white/80 backdrop-blur">
        <div className="mx-auto flex max-w-3xl items-center justify-between px-4 py-3">
          <div className="flex items-center gap-2.5">
            <div className="flex h-9 w-9 items-center justify-center rounded-xl bg-gradient-to-br from-indigo-500 to-violet-600 text-sm font-black text-white">
              OKF
            </div>
            <div>
              <h1 className="text-sm font-bold text-slate-800">Conversor de Documentos</h1>
              <p className="text-xs text-slate-400">MD · TXT · HTML · PDF · DOCX · EPUB → Markdown</p>
            </div>
          </div>
          <button
            onClick={logout}
            className="rounded-lg border border-slate-300 px-3 py-1.5 text-xs font-semibold text-slate-600 transition hover:bg-slate-100"
          >
            Salir
          </button>
        </div>
      </header>

      <main className="mx-auto max-w-3xl space-y-6 px-4 py-8">
        <Upload onUploaded={() => void load()} />

        <Metrics refreshKey={refreshKey} />

        {error && (
          <div className="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
            {error}
          </div>
        )}

        <section>
          <div className="mb-3 flex items-center justify-between">
            <h2 className="text-sm font-bold uppercase tracking-wide text-slate-500">
              Mis trabajos
            </h2>
            {active > 0 ? (
              <span className="flex items-center gap-2 rounded-full bg-blue-100 px-3 py-1 text-xs font-semibold text-blue-700">
                <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-blue-500" />
                {active} en progreso
              </span>
            ) : (
              <button
                onClick={() => void load()}
                className="text-xs font-semibold text-indigo-600 hover:underline"
              >
                Refrescar
              </button>
            )}
          </div>
          <JobList jobs={jobs} onRetried={() => void load()} />
        </section>
      </main>
    </div>
  )
}