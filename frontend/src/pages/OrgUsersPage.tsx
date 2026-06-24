import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../hooks/useApi'
import { useAuthStore } from '../stores/useAuthStore'
import { ApiError } from '../api/client'
import type { UserResponse } from '../api/types'
import { useToastStore } from '../stores/useToastStore'

const ALL_SCOPES = [
  'workflow:read',
  'workflow:write',
  'workflow:run',
  'eval:read',
  'eval:write',
  'eval:run',
]

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

  const load = () => {
    setLoading(true)
    api.listOrgUsers()
      .then(r => setUsers(r.users))
      .catch(err => setError(err instanceof ApiError ? err.message : 'Failed to load users'))
      .finally(() => setLoading(false))
  }

  useEffect(() => { load() }, [])

  const handleInvite = async (e: React.FormEvent) => {
    e.preventDefault()
    setInviting(true)
    try {
      const res = await api.inviteUser({ email: inviteEmail, role: inviteRole })
      setInviteToken(res.token)
      setInviteEmail('')
      addToast('success', `Invitation created for ${res.email}`)
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
              {inviting ? 'Inviting…' : 'Send invite'}
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

        {/* Users table */}
        <section className="bg-gray-900 border border-gray-700 rounded-xl overflow-hidden">
          <h2 className="text-sm font-semibold text-gray-300 px-5 py-4 border-b border-gray-700">
            Members
          </h2>

          {loading && <p className="text-sm text-gray-400 p-5">Loading…</p>}
          {error && <p className="text-sm text-red-400 p-5">{error}</p>}

          {!loading && !error && (
            <div className="divide-y divide-gray-800">
              {users.map(u => (
                <div key={u.id} className="px-5 py-4">
                  <div className="flex items-center gap-4 flex-wrap">
                    <div className="flex-1 min-w-0">
                      <p className="text-sm font-medium text-gray-200 truncate">{u.email}</p>
                      <p className="text-xs text-gray-500">{u.id}</p>
                    </div>

                    {/* Role selector — only for other users */}
                    {u.id !== currentUser?.id ? (
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
                    ) : (
                      <span className="text-xs px-2 py-1 rounded bg-gray-700 text-gray-300">{u.role}</span>
                    )}

                    {/* Remove button */}
                    {u.id !== currentUser?.id && (
                      <button
                        onClick={() => handleRemove(u.id, u.email)}
                        className="text-xs text-red-500 hover:text-red-400 transition-colors"
                      >
                        Remove
                      </button>
                    )}
                  </div>

                  {/* Permission toggles — only for members */}
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
              ))}
            </div>
          )}
        </section>
      </div>
    </div>
  )
}
