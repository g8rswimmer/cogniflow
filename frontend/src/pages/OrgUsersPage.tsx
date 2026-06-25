import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../hooks/useApi'
import { useAuthStore } from '../stores/useAuthStore'
import { ApiError } from '../api/client'
import type { UserResponse, OrgEmailSettingsResponse } from '../api/types'
import { useToastStore } from '../stores/useToastStore'

const ALL_SCOPES = [
  'workflow:read',
  'workflow:write',
  'workflow:run',
  'eval:read',
  'eval:write',
  'eval:run',
]

const DEFAULT_PORT = '587'

export function OrgUsersPage() {
  const currentUser = useAuthStore(s => s.user)
  const addToast = useToastStore(s => s.addToast)
  const [users, setUsers] = useState<UserResponse[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [inviteEmail, setInviteEmail] = useState('')
  const [inviteRole, setInviteRole] = useState<'member' | 'org_admin'>('member')
  const [inviting, setInviting] = useState(false)
  const [inviteToken, setInviteToken] = useState<string | null>(null)

  // Email settings state
  const [emailSettings, setEmailSettings] = useState<OrgEmailSettingsResponse | null>(null)
  const [emailSettingsOpen, setEmailSettingsOpen] = useState(false)
  const [emailSettingsLoading, setEmailSettingsLoading] = useState(false)
  const [smtpHost, setSmtpHost] = useState('')
  const [smtpPort, setSmtpPort] = useState(DEFAULT_PORT)
  const [smtpUser, setSmtpUser] = useState('')
  const [smtpPassword, setSmtpPassword] = useState('')
  const [smtpFrom, setSmtpFrom] = useState('')
  const [templateSubject, setTemplateSubject] = useState('')
  const [templateBody, setTemplateBody] = useState('')
  const [savingEmailSettings, setSavingEmailSettings] = useState(false)

  const load = () => {
    setLoading(true)
    api.listOrgUsers()
      .then(r => setUsers(r.users))
      .catch(err => setError(err instanceof ApiError ? err.message : 'Failed to load users'))
      .finally(() => setLoading(false))
  }

  const loadEmailSettings = async () => {
    setEmailSettingsLoading(true)
    try {
      const s = await api.getOrgEmailSettings()
      setEmailSettings(s)
      setSmtpHost(s.smtp_host)
      setSmtpPort(s.smtp_port || DEFAULT_PORT)
      setSmtpUser(s.smtp_user)
      setSmtpPassword(s.smtp_password) // "***" when set
      setSmtpFrom(s.smtp_from)
      setTemplateSubject(s.subject)
      setTemplateBody(s.body)
    } catch (err) {
      addToast('error', err instanceof ApiError ? err.message : 'Failed to load email settings')
    } finally {
      setEmailSettingsLoading(false)
    }
  }

  useEffect(() => { load() }, [])

  const openEmailSettings = () => {
    setEmailSettingsOpen(true)
    if (!emailSettings) loadEmailSettings()
  }

  const handleSaveEmailSettings = async (e: React.FormEvent) => {
    e.preventDefault()
    setSavingEmailSettings(true)
    try {
      const saved = await api.upsertOrgEmailSettings({
        smtp_host: smtpHost,
        smtp_port: smtpPort || DEFAULT_PORT,
        smtp_user: smtpUser,
        smtp_password: smtpPassword,
        smtp_from: smtpFrom,
        subject: templateSubject,
        body: templateBody,
      })
      setEmailSettings(saved)
      addToast('success', 'Email settings saved')
    } catch (err) {
      addToast('error', err instanceof ApiError ? err.message : 'Failed to save email settings')
    } finally {
      setSavingEmailSettings(false)
    }
  }

  const handleResetEmailSettings = async () => {
    if (!confirm('Reset all email settings and template to defaults?')) return
    try {
      await api.deleteOrgEmailSettings()
      await loadEmailSettings()
      addToast('success', 'Email settings reset to defaults')
    } catch (err) {
      addToast('error', err instanceof ApiError ? err.message : 'Failed to reset settings')
    }
  }

  const handleInvite = async (e: React.FormEvent) => {
    e.preventDefault()
    setInviting(true)
    try {
      const res = await api.inviteUser({ email: inviteEmail, role: inviteRole })
      setInviteEmail('')
      if (res.email_sent) {
        setInviteToken(null)
        addToast('success', `Invitation sent to ${res.email}`)
      } else {
        setInviteToken(res.token)
        addToast('success', `Invitation created for ${res.email}`)
      }
    } catch (err) {
      addToast('error', err instanceof ApiError ? err.message : 'Failed to create invitation')
    } finally {
      setInviting(false)
    }
  }

  const handleRoleChange = async (userId: string, role: string) => {
    try {
      await api.updateOrgUserRole(userId, role)
      load()
    } catch (err) {
      addToast('error', err instanceof ApiError ? err.message : 'Failed to update role')
    }
  }

  const handlePermissionToggle = async (user: UserResponse, scope: string) => {
    const next = user.permissions.includes(scope)
      ? user.permissions.filter(p => p !== scope)
      : [...user.permissions, scope]
    try {
      await api.updateOrgUserPermissions(user.id, next)
      load()
    } catch (err) {
      addToast('error', err instanceof ApiError ? err.message : 'Failed to update permissions')
    }
  }

  const handleRemove = async (userId: string, email: string) => {
    if (!confirm(`Remove ${email}?`)) return
    try {
      await api.removeOrgUser(userId)
      load()
      addToast('success', `Removed ${email}`)
    } catch (err) {
      addToast('error', err instanceof ApiError ? err.message : 'Failed to remove user')
    }
  }

  const emailConfigured = emailSettings?.smtp_configured ?? false
  const inviteButtonLabel = inviting ? 'Sending…' : emailConfigured ? 'Send invite' : 'Generate link'

  return (
    <div className="min-h-screen bg-gray-950 text-gray-100">
      <header className="h-12 flex items-center px-4 gap-3 bg-gray-900 border-b border-gray-700">
        <Link to="/" className="text-gray-400 hover:text-gray-200 text-sm">← Workflows</Link>
        <div className="w-px h-5 bg-gray-700" />
        <h1 className="text-sm font-semibold">Organisation Users</h1>
        {currentUser && (
          <span className="ml-auto text-xs text-gray-400">{currentUser.org_name}</span>
        )}
      </header>

      <div className="max-w-4xl mx-auto p-6 flex flex-col gap-8">

        {/* Invite form */}
        <section className="bg-gray-900 border border-gray-700 rounded-xl p-5">
          <h2 className="text-sm font-semibold text-gray-300 mb-4">Invite user</h2>
          <form onSubmit={handleInvite} className="flex gap-3 flex-wrap">
            <input
              type="email"
              required
              value={inviteEmail}
              onChange={e => setInviteEmail(e.target.value)}
              placeholder="user@example.com"
              className="flex-1 min-w-48 bg-gray-800 border border-gray-600 rounded-md px-3 py-1.5 text-sm text-gray-100 focus:outline-none focus:border-indigo-500"
            />
            <select
              value={inviteRole}
              onChange={e => setInviteRole(e.target.value as 'member' | 'org_admin')}
              className="bg-gray-800 border border-gray-600 rounded-md px-2 py-1.5 text-sm text-gray-100 focus:outline-none"
            >
              <option value="member">Member</option>
              <option value="org_admin">Org Admin</option>
            </select>
            <button
              type="submit"
              disabled={inviting}
              className="bg-indigo-600 hover:bg-indigo-500 disabled:opacity-50 text-white text-xs font-semibold px-4 py-1.5 rounded-md transition-colors"
            >
              {inviteButtonLabel}
            </button>
          </form>

          {inviteToken && (
            <div className="mt-3 bg-gray-800 border border-gray-600 rounded-md p-3">
              <p className="text-xs text-gray-400 mb-1">Share this invite link:</p>
              <code className="text-xs text-indigo-300 break-all">
                {window.location.origin}/invite/{inviteToken}
              </code>
              <button
                onClick={() => navigator.clipboard.writeText(`${window.location.origin}/invite/${inviteToken}`)}
                className="mt-2 text-xs text-gray-400 hover:text-gray-200 block"
              >
                Copy link
              </button>
            </div>
          )}
        </section>

        {/* Email & SMTP Settings */}
        <section className="bg-gray-900 border border-gray-700 rounded-xl overflow-hidden">
          <button
            onClick={() => emailSettingsOpen ? setEmailSettingsOpen(false) : openEmailSettings()}
            className="w-full flex items-center justify-between px-5 py-4 text-left hover:bg-gray-800 transition-colors"
          >
            <span className="text-sm font-semibold text-gray-300">Email settings</span>
            <div className="flex items-center gap-3">
              {emailSettings && (
                <span className={`text-xs px-2 py-0.5 rounded-full border ${
                  emailSettings.smtp_configured
                    ? 'border-emerald-600 text-emerald-400'
                    : 'border-gray-600 text-gray-500'
                }`}>
                  {emailSettings.smtp_configured ? 'SMTP configured' : 'Not configured'}
                </span>
              )}
              <span className="text-gray-500 text-xs">{emailSettingsOpen ? '▲' : '▼'}</span>
            </div>
          </button>

          {emailSettingsOpen && (
            <div className="border-t border-gray-700 p-5">
              {emailSettingsLoading && <p className="text-sm text-gray-400">Loading…</p>}

              {!emailSettingsLoading && (
                <form onSubmit={handleSaveEmailSettings} className="flex flex-col gap-5">

                  {/* SMTP credentials */}
                  <div>
                    <h3 className="text-xs font-semibold text-gray-400 uppercase tracking-wider mb-3">SMTP credentials</h3>
                    <div className="grid grid-cols-2 gap-3">
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
                        <label className="block text-xs text-gray-400 mb-1">Username</label>
                        <input
                          value={smtpUser}
                          onChange={e => setSmtpUser(e.target.value)}
                          placeholder="user@example.com"
                          autoComplete="off"
                          className="w-full bg-gray-800 border border-gray-600 rounded-md px-3 py-1.5 text-sm text-gray-100 font-mono focus:outline-none focus:border-indigo-500"
                        />
                      </div>
                      <div className="col-span-2 sm:col-span-1">
                        <label className="block text-xs text-gray-400 mb-1">
                          Password
                          {smtpPassword === '***' && (
                            <span className="ml-2 text-gray-500">(set — clear to remove)</span>
                          )}
                        </label>
                        <input
                          type="password"
                          value={smtpPassword}
                          onChange={e => setSmtpPassword(e.target.value)}
                          placeholder="••••••••"
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
                  </div>

                  {/* Email template */}
                  <div>
                    <h3 className="text-xs font-semibold text-gray-400 uppercase tracking-wider mb-3">Invite email template</h3>
                    <div className="text-xs text-gray-500 bg-gray-800 border border-gray-700 rounded-md p-3 font-mono mb-3 leading-relaxed">
                      Available variables:{' '}
                      {['{{.OrgName}}', '{{.InviteURL}}', '{{.InviteeEmail}}', '{{.InviterEmail}}', '{{.ExpiresAt.Format "Jan 2, 2006"}}'].map(v => (
                        <span key={v} className="text-indigo-400 mr-2">{v}</span>
                      ))}
                    </div>
                    <div className="flex flex-col gap-3">
                      <div>
                        <label className="block text-xs text-gray-400 mb-1">Subject</label>
                        <input
                          value={templateSubject}
                          onChange={e => setTemplateSubject(e.target.value)}
                          placeholder="You've been invited to join {{.OrgName}}"
                          className="w-full bg-gray-800 border border-gray-600 rounded-md px-3 py-1.5 text-sm text-gray-100 font-mono focus:outline-none focus:border-indigo-500"
                        />
                      </div>
                      <div>
                        <label className="block text-xs text-gray-400 mb-1">Body</label>
                        <textarea
                          rows={10}
                          value={templateBody}
                          onChange={e => setTemplateBody(e.target.value)}
                          className="w-full bg-gray-800 border border-gray-600 rounded-md px-3 py-2 text-sm text-gray-100 font-mono focus:outline-none focus:border-indigo-500 resize-y"
                        />
                      </div>
                    </div>
                  </div>

                  <div className="flex gap-3">
                    <button
                      type="submit"
                      disabled={savingEmailSettings}
                      className="bg-indigo-600 hover:bg-indigo-500 disabled:opacity-50 text-white text-xs font-semibold px-4 py-1.5 rounded-md transition-colors"
                    >
                      {savingEmailSettings ? 'Saving…' : 'Save settings'}
                    </button>
                    {emailSettings && !emailSettings.is_default && (
                      <button
                        type="button"
                        onClick={handleResetEmailSettings}
                        className="text-xs text-gray-400 hover:text-gray-200 px-4 py-1.5 border border-gray-700 hover:border-gray-500 rounded-md transition-colors"
                      >
                        Reset to defaults
                      </button>
                    )}
                  </div>
                </form>
              )}
            </div>
          )}
        </section>

        {/* Users table */}
        <section className="bg-gray-900 border border-gray-700 rounded-xl overflow-hidden">
          <h2 className="text-sm font-semibold text-gray-300 px-5 py-4 border-b border-gray-700">
            Members
          </h2>

          {loading && <p className="text-sm text-gray-400 p-5">Loading…</p>}
          {error && <p className="text-sm text-red-400 p-5">{error}</p>}

          {!loading && !error && (
            <div className="divide-y divide-gray-800">
              {users.map(u => {
                const isProtected = u.role === 'system_admin' && currentUser?.role !== 'system_admin'
                return (
                <div key={u.id} className="px-5 py-4">
                  <div className="flex items-center gap-4 flex-wrap">
                    <div className="flex-1 min-w-0">
                      <p className="text-sm font-medium text-gray-200 truncate">{u.email}</p>
                      <p className="text-xs text-gray-500">{u.id}</p>
                    </div>

                    {u.id === currentUser?.id || isProtected ? (
                      <span className={`text-xs px-2 py-1 rounded border ${
                        u.role === 'system_admin'
                          ? 'bg-amber-900/30 border-amber-700 text-amber-400'
                          : 'bg-gray-700 border-gray-600 text-gray-300'
                      }`}>
                        {u.role === 'system_admin' ? 'System Admin' : u.role}
                      </span>
                    ) : (
                      <select
                        value={u.role}
                        onChange={e => handleRoleChange(u.id, e.target.value)}
                        className="bg-gray-800 border border-gray-600 rounded px-2 py-1 text-xs text-gray-200 focus:outline-none"
                      >
                        <option value="member">Member</option>
                        <option value="org_admin">Org Admin</option>
                        {currentUser?.role === 'system_admin' && (
                          <option value="system_admin">System Admin</option>
                        )}
                      </select>
                    )}

                    {u.id !== currentUser?.id && !isProtected && (
                      <button
                        onClick={() => handleRemove(u.id, u.email)}
                        className="text-xs text-red-500 hover:text-red-400 transition-colors"
                      >
                        Remove
                      </button>
                    )}
                  </div>

                  {u.role === 'member' && (
                    <div className="mt-3 flex flex-wrap gap-2">
                      {ALL_SCOPES.map(scope => (
                        <button
                          key={scope}
                          onClick={() => handlePermissionToggle(u, scope)}
                          className={`text-xs px-2.5 py-1 rounded-full border transition-colors ${
                            u.permissions.includes(scope)
                              ? 'bg-indigo-700 border-indigo-500 text-indigo-100'
                              : 'bg-gray-800 border-gray-600 text-gray-400 hover:border-gray-400'
                          }`}
                        >
                          {scope}
                        </button>
                      ))}
                    </div>
                  )}
                </div>
              )})}
            </div>
          )}
        </section>
      </div>
    </div>
  )
}
