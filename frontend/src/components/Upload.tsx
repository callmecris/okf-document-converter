import { useRef, useState } from 'react'
import { uploadJob, MAX_UPLOAD_MB, type Job } from '../api'

const ACCEPT = '.md,.txt,.html,.pdf,.docx,.epub'
const MAX_BYTES = MAX_UPLOAD_MB * 1024 * 1024

type Props = {
  onUploaded: (job: Job) => void
}

export default function Upload({ onUploaded }: Props) {
  const [file, setFile] = useState<File | null>(null)
  const [dragging, setDragging] = useState(false)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)

  const pick = async () => {
    if (!file) return
    if (file.size > MAX_BYTES) {
      setError(`El archivo supera el tamaño máximo permitido (${MAX_UPLOAD_MB} MB).`)
      return
    }
    setBusy(true)
    setError('')
    try {
      const job = await uploadJob(file)
      setFile(null)
      if (inputRef.current) inputRef.current.value = ''
      onUploaded(job)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Error al subir el archivo')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div
      onDragOver={(e) => {
        e.preventDefault()
        setDragging(true)
      }}
      onDragLeave={() => setDragging(false)}
      onDrop={(e) => {
        e.preventDefault()
        setDragging(false)
        const dropped = e.dataTransfer.files?.[0]
        if (dropped && ACCEPT.split(',').some((ext) => dropped.name.toLowerCase().endsWith(ext))) {
          setFile(dropped)
        } else {
          setError('Solo se admiten archivos MD, TXT, HTML, PDF, DOCX o EPUB')
        }
      }}
      className={`rounded-2xl border-2 border-dashed p-6 text-center transition ${
        dragging
          ? 'border-indigo-400 bg-indigo-50'
          : 'border-slate-200 bg-white hover:border-indigo-300'
      }`}
    >
      <input
        ref={inputRef}
        type="file"
        accept={ACCEPT}
        className="hidden"
        onChange={(e) => setFile(e.target.files?.[0] ?? null)}
      />
      <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-xl bg-indigo-100 text-indigo-500">
        <svg
          xmlns="http://www.w3.org/2000/svg"
          fill="none"
          viewBox="0 0 24 24"
          strokeWidth={1.5}
          stroke="currentColor"
          className="h-6 w-6"
        >
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            d="M19.5 14.25v-2.625a3.375 3.375 0 0 0-3.375-3.375h-1.5A1.125 1.125 0 0 1 13.5 7.125v-1.5a3.375 3.375 0 0 0-3.375-3.375H8.25m.75 12 3 3m0 0 3-3m-3 3v-6m-1.5-9H5.625c-.621 0-1.125.504-1.125 1.125v17.25c0 .621.504 1.125 1.125 1.125h12.75c.621 0 1.125-.504 1.125-1.125V11.25a9 9 0 0 0-9-9Z"
          />
        </svg>
      </div>
      <p className="mt-2 text-sm font-medium text-slate-700">
        {file ? file.name : 'Arrastra tu documento aquí'}
      </p>
      <p className="mt-1 text-xs text-slate-400">
        {file
          ? `${(file.size / 1024 / 1024).toFixed(2)} MB`
          : `MD, TXT, HTML, PDF, DOCX o EPUB · máx. ${MAX_UPLOAD_MB} MB`}
      </p>
      <div className="mt-4 flex justify-center gap-3">
        <button
          type="button"
          onClick={() => inputRef.current?.click()}
          className="rounded-lg border border-slate-300 px-4 py-2 text-sm font-semibold text-slate-700 transition hover:bg-slate-50"
        >
          Elegir archivo
        </button>
        {file && (
          <button
            type="button"
            onClick={() => void pick()}
            disabled={busy}
            className="rounded-lg bg-indigo-600 px-4 py-2 text-sm font-semibold text-white transition hover:bg-indigo-700 disabled:opacity-50"
          >
            {busy ? 'Subiendo…' : 'Convertir'}
          </button>
        )}
      </div>
      {error && <p className="mt-3 text-xs text-red-600">{error}</p>}
    </div>
  )
}