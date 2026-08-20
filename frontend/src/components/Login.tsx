import { useState } from 'react'
import { login, register } from '../api'

type Props = {
  onAuth: (token: string) => void
}

export default function Login({ onAuth }: Props) {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const submit = async (kind: 'login' | 'register') => {
    setError('')
    setLoading(true)
    try {
      const res = kind === 'login' ? await login(email, password) : await register(email, password)
      onAuth(res.token)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Error de autenticación')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-slate-950 px-4">
      <div className="w-full max-w-md">
        <div className="mb-8 text-center">
          <div className="mx-auto mb-4 flex h-14 w-14 items-center justify-center rounded-2xl bg-gradient-to-br from-indigo-500 to-violet-600 text-2xl font-black text-white shadow-lg shadow-indigo-500/30">
            OKF
          </div>
          <h1 className="text-2xl font-bold tracking-tight text-white">Conversor de Documentos</h1>
          <p className="mt-1 text-sm text-slate-400">MD · TXT · HTML · PDF · DOCX · EPUB → Bundles Markdown</p>
        </div>

        <div className="rounded-2xl bg-white p-6 shadow-xl">
          <div className="mb-4">
            <label className="mb-1 block text-sm font-medium text-slate-700">Email</label>
            <input
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="tu@email.com"
              className="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm outline-none transition focus:border-indigo-500 focus:ring-2 focus:ring-indigo-200"
            />
          </div>
          <div className="mb-5">
            <label className="mb-1 block text-sm font-medium text-slate-700">Contraseña</label>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="Mínimo 8 caracteres"
              className="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm outline-none transition focus:border-indigo-500 focus:ring-2 focus:ring-indigo-200"
            />
          </div>

          {error && (
            <div className="mb-4 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-600">
              {error}
            </div>
          )}

          <div className="flex gap-3">
            <button
              onClick={() => void submit('login')}
              disabled={loading}
              className="flex-1 rounded-lg bg-indigo-600 py-2 text-sm font-semibold text-white transition hover:bg-indigo-700 disabled:opacity-50"
            >
              {loading ? 'Espera…' : 'Ingresar'}
            </button>
            <button
              onClick={() => void submit('register')}
              disabled={loading}
              className="flex-1 rounded-lg border border-slate-300 py-2 text-sm font-semibold text-slate-700 transition hover:bg-slate-50 disabled:opacity-50"
            >
              Registrarse
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}