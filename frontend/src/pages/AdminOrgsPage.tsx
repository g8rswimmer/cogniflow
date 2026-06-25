import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../hooks/useApi'
import { ApiError } from '../api/client'
import type { OrgResponse } from '../api/types'
import { useToastStore } from '../stores/useToastStore'

export function AdminOrgsPage() {
  const addToast = useToastStore(s => s.addToast)
  const [orgs, setOrgs] = useState<OrgResponse[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const [showCreate, setShowCreate] = useState(false)
  const [orgName, setOrgName] = useState('')
  const [adminEmail, setAdminEmail] = useState('')
  const [adminPassword, setAdminPassword] = useState('')
  const [creating, setCreating] = useState(false)

  // Optional email settings for new org
  const [showEmailSettings, setShowEmailSettings] = useState(false)
  const [smtpHost, setSmtpHost] = useState('')
  const [smtpPort, setSmtpPort] = useState('587')
  const [smtpUser, setSmtpUser] = useState('')
  const [smtpPassword, setSmtpPassword] = useState('')
  const [smtpFrom, setSmtpFrom] = useState('')

  const load = () => {
    setLoading(true)
    api.listOrgs()
      .then(r => setOrgs(r.organizations))
      .catch(err => setError(err instanceof ApiError ? err.message : 'Failed to load orgs'))
      .finally(() => setLoading(false))
  }

  useEffect(() => { load() }, [])

  const resetCreateForm = () => {
    setOrgName(''); setAdminEmail(''); setAdminPassword('')
    setSmtpHost(''); setSmtpPort('587'); setSmtpUser(''); setSmtpPassword(''); setSmtpFrom('')
    setShowEmailSettings(false)
  }

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    setCreating(true)
    try {
      const res = await api.createOrg({ name: orgName, admin_email: adminEmail, admin_password: adminPassword })
      if (smtpHost) {
        try {
          await api.setOrgEmailSettingsAdmin(res.organization.id, {
            smtp_host: smtpHost, smtp_port: smtpPort || '587',
            smtp_user: smtpUser, smtp_password: smtpPassword,
            smtp_from: smtpFrom, subject: '', body: '',
          })
        } catch {
          addToast('error', 'Org created but email settings failed — configure them from the Org Users page')
        }
      }
      addToast('success', `Created org "${res.organization.name}"`)
      setShowCreate(false)
      resetCreateForm()
      load()
    } catch (err) {
      addToast('error', err instanceof ApiError ? err.message : 'Failed to create org')
    } finally {
      setCreating(false)
    }
  }

  const handleDelete = async (orgId: string, name: string) => {
    if (!confirm(`Delete org "${name}" and ALL its data? This cannot be undone.`)) return
    try {
      await api.deleteOrg(orgId)
      addToast('success', `Deleted org "${name}"`)
      load()
    } catch (err) {
      addToast('error', err instanceof ApiError ? err.message : 'Failed to delete org')
    }
  }

  return (
    <div className="min-h-screen bg-gray-950 text-gray-100">
      <header className="h-12 flex items-center px-4 gap-3 bg-gray-900 border-b border-gray-700">
        <Link to="/" className="text-gray-400 hover:text-gray-200 text-sm">← Workflows</Link>
        <div className="w-px h-5 bg-gray-700" />
        <h1 className="text-sm font-semibold">System Admin — Organisations</h1>
        <button
          onClick={() => setShowCreate(s => !s)}
          className="ml-auto bg-indigo-600 hover:bg-indigo-500 text-white text-xs font-semibold px-3 py-1.5 rounded-md transition-colors"
        >
          + New org
        </button>
      </header>

      <div className="max-w-3xl mx-auto p-6 flex flex-col gap-6">

        {/* Create form */}
        {showCreate && (
          <section className="bg-gray-900 border border-gray-700 rounded-xl p-5">
            <h2 className="text-sm font-semibold text-gray-300 mb-4">Create organisation</h2>
            <form onSubmit={handleCreate} className="flex flex-col gap-3">
              <div>
                <label className="block text-xs text-gray-400 mb-1">Organisation name</label>
                <input
                  required
                  value={orgName}
                  onChange={e => setOrgName(e.target.value)}
                  className="w-full bg-gray-800 border border-gray-600 rounded-md px-3 py-1.5 text-sm text-gray-100 focus:outline-none focus:border-indigo-500"
                  placeholder="Acme Corp"
                />
              </div>
              <div>
                <label className="block text-xs text-gray-400 mb-1">Admin email</label>
                <input
                  type="email"
                  required
                  value={adminEmail}
                  onChange={e => setAdminEmail(e.target.value)}
                  className="w-full bg-gray-800 border border-gray-600 rounded-md px-3 py-1.5 text-sm text-gray-100 focus:outline-none focus:border-indigo-500"
                  placeholder="admin@acme.com"
                />
              </div>
              <div>
                <label className="block text-xs text-gray-400 mb-1">Admin password</label>
                <input
                  type="password"
                  required
                  minLength={8}
                  value={adminPassword}
                  onChange={e => setAdminPassword(e.target.value)}
                  className="w-full bg-gray-800 border border-gray-600 rounded-md px-3 py-1.5 text-sm text-gray-100 focus:outline-none focus:border-indigo-500"
                  placeholder="Min 8 characters"
                />
              </div>
              {/* Optional SMTP settings */}
              <div className="border-t border-gray-700 pt-3">
                <button
                  type="button"
                  onClick={() => setShowEmailSettings(s => !s)}
                  className="text-xs text-gray-400 hover:text-gray-200 flex items-center gap-1.5"
                >
                  <span>{showEmailSettings ? '▼' : '▶'}</span>
                  Email settings (optional)
                </button>

                {showEmailSettings && (
                  <div className="mt-3 grid grid-cols-2 gap-3">
                    <div className="col-span-2 sm:col-span-1">
                      <label className="block text-xs text-gray-400 mb-1">SMTP host</label>
                      <input
                        value={smtpHost}
                        onChange={e => setSmtpHost(e.target.value)}
                        placeholder="smtp.example.com"
                        className="w-full bg-gray-800 border border-gray-600 rounded-md px-3 py-1.5 text-sm text-gray-100 font-mono focus:outline-none focus:border-indigo-500"
                      />
                    </div>
                    <div className="col-span-2 sm:col-span-1">
                      <label className="block text-xs text-gray-400 mb-1">Port</label>
                      <input
                        value={smtpPort}
                        onChange={e => setSmtpPort(e.target.value)}
                        placeholder="587"
                        className="w-full bg-gray-800 border border-gray-600 rounded-md px-3 py-1.5 text-sm text-gray-100 font-mono focus:outline-none focus:border-indigo-500"
                      />
                    </div>
                    <div className="col-span-2 sm:col-span-1">
                      <label className="block text-xs text-gray-400 mb-1">SMTP username</label>
                      <input
                        value={smtpUser}
                        onChange={e => setSmtpUser(e.target.value)}
                        autoComplete="off"
                        className="w-full bg-gray-800 border border-gray-600 rounded-md px-3 py-1.5 text-sm text-gray-100 font-mono focus:outline-none focus:border-indigo-500"
                      />
                    </div>
                    <div className="col-span-2 sm:col-span-1">
                      <label className="block text-xs text-gray-400 mb-1">SMTP password</label>
                      <input
                        type="password"
                        value={smtpPassword}
                        onChange={e => setSmtpPassword(e.target.value)}
                        autoComplete="new-password"
                        className="w-full bg-gray-800 border border-gray-600 rounded-md px-3 py-1.5 text-sm text-gray-100 font-mono focus:outline-none focus:border-indigo-500"
                      />
                    </div>
                    <div className="col-span-2">
                      <label className="block text-xs text-gray-400 mb-1">From address</label>
                      <input
                        type="email"
                        value={smtpFrom}
                        onChange={e => setSmtpFrom(e.target.value)}
                        placeholder="no-reply@example.com"
                        className="w-full bg-gray-800 border border-gray-600 rounded-md px-3 py-1.5 text-sm text-gray-100 font-mono focus:outline-none focus:border-indigo-500"
                      />
                    </div>
                  </div>
                )}
              </div>

              <div className="flex gap-3 mt-1">
                <button
                  type="submit"
                  disabled={creating}
                  className="bg-indigo-600 hover:bg-indigo-500 disabled:opacity-50 text-white text-xs font-semibold px-4 py-1.5 rounded-md transition-colors"
                >
                  {creating ? 'Creating…' : 'Create'}
                </button>
                <button
                  type="button"
                  onClick={() => { setShowCreate(false); resetCreateForm() }}
                  className="text-xs text-gray-400 hover:text-gray-200 px-4 py-1.5"
                >
                  Cancel
                </button>
              </div>
            </form>
          </section>
        )}

        {/* Org list */}
        <section className="bg-gray-900 border border-gray-700 rounded-xl overflow-hidden">
          {loading && <p className="text-sm text-gray-400 p-5">Loading…</p>}
          {error && <p className="text-sm text-red-400 p-5">{error}</p>}

          {!loading && !error && (
            <div className="divide-y divide-gray-800">
              {orgs.length === 0 && (
                <p className="text-sm text-gray-500 p-5">No organisations yet.</p>
              )}
              {orgs.map(org => (
                <div key={org.id} className="flex items-center gap-4 px-5 py-3">
                  <div className="flex-1 min-w-0">
                    <p className="text-sm font-medium text-gray-200">{org.name}</p>
                    <p className="text-xs text-gray-500">{org.id}</p>
                  </div>
                  <p className="text-xs text-gray-500">
                    {new Date(org.created_at).toLocaleDateString()}
                  </p>
                  <button
                    onClick={() => handleDelete(org.id, org.name)}
                    className="text-xs text-red-500 hover:text-red-400 transition-colors"
                  >
                    Delete
                  </button>
                </div>
              ))}
            </div>
          )}
        </section>
      </div>
    </div>
  )
}
