import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { api } from '../hooks/useApi'
import { useAuthStore } from '../stores/useAuthStore'
import { ApiError } from '../api/client'

export function AcceptInvitePage() {
  const { token } = useParams<{ token: string }>()
  const navigate = useNavigate()
  const login = useAuthStore(s => s.login)

  const [preview, setPreview] = useState<{ email: string; role: string; org_name: string } | null>(null)
  const [previewError, setPreviewError] = useState<string | null>(null)
  const [password, setPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [submitError, setSubmitError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    if (!token) return
    api.getInvite(token)
      .then(setPreview)
      .catch(err => setPreviewError(err instanceof ApiError ? err.message : 'Invalid invitation'))
  }, [token])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (password !== confirmPassword) {
      setSubmitError('Passwords do not match')
      return
    }
    if (!token) return
    setSubmitError(null)
    setLoading(true)
    try {
      const { token: jwt, user } = await api.acceptInvite(token, password)
      login(jwt, user)
      navigate('/', { replace: true })
    } catch (err) {
      setSubmitError(err instanceof ApiError ? err.message : 'Failed to accept invitation')
    } finally {
      setLoading(false)
    }
  }

  if (previewError) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gray-950">
        <div className="bg-gray-900 border border-red-800 rounded-xl p-8 max-w-sm w-full text-center">
          <p className="text-red-400 text-sm">{previewError}</p>
        </div>
      </div>
    )
  }

  if (!preview) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gray-950">
        <p className="text-gray-400 text-sm">Loading invitation…</p>
      </div>
    )
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-950">
      <div className="w-full max-w-sm bg-gray-900 border border-gray-700 rounded-xl p-8 shadow-xl">
        <h1 className="text-xl font-bold text-gray-100 mb-2 text-center">Accept invitation</h1>
        <p className="text-sm text-gray-400 text-center mb-6">
          You've been invited to <span className="text-gray-200 font-medium">{preview.org_name}</span> as{' '}
          <span className="text-gray-200 font-medium">{preview.role}</span>
          <br />
          <span className="text-indigo-400">{preview.email}</span>
        </p>

        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <div>
            <label className="block text-xs font-medium text-gray-400 mb-1">Password</label>
            <input
              type="password"
              required
              autoFocus
              minLength={8}
              value={password}
              onChange={e => setPassword(e.target.value)}
              className="w-full bg-gray-800 border border-gray-600 rounded-md px-3 py-2 text-sm text-gray-100 focus:outline-none focus:border-indigo-500"
              placeholder="Choose a password (min 8 chars)"
            />
          </div>

          <div>
            <label className="block text-xs font-medium text-gray-400 mb-1">Confirm password</label>
            <input
              type="password"
              required
              value={confirmPassword}
              onChange={e => setConfirmPassword(e.target.value)}
              className="w-full bg-gray-800 border border-gray-600 rounded-md px-3 py-2 text-sm text-gray-100 focus:outline-none focus:border-indigo-500"
              placeholder="Re-enter password"
            />
          </div>

          {submitError && (
            <p className="text-sm text-red-400 bg-red-900/30 border border-red-800 rounded-md px-3 py-2">
              {submitError}
            </p>
          )}

          <button
            type="submit"
            disabled={loading}
            className="mt-2 w-full bg-indigo-600 hover:bg-indigo-500 disabled:opacity-50 text-white font-semibold text-sm rounded-md py-2 transition-colors"
          >
            {loading ? 'Creating account…' : 'Create account'}
          </button>
        </form>
      </div>
    </div>
  )
}
