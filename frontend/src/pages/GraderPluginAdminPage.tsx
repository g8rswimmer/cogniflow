import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../hooks/useApi'
import type { GraderRegistration } from '../api/types'
import { ApiError } from '../api/client'

export function GraderPluginAdminPage() {
  const [plugins, setPlugins] = useState<GraderRegistration[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  // Register form
  const [newAddress, setNewAddress] = useState('')
  const [registering, setRegistering] = useState(false)
  const [registerError, setRegisterError] = useState<string | null>(null)

  // Update form
  const [editingTypeId, setEditingTypeId] = useState<string | null>(null)
  const [editAddress, setEditAddress] = useState('')
  const [updating, setUpdating] = useState(false)
  const [updateError, setUpdateError] = useState<string | null>(null)

  const load = async () => {
    setLoading(true)
    setError(null)
    try {
      const { grader_plugins } = await api.listGraderPlugins()
      setPlugins(grader_plugins ?? [])
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load grader plugins')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { load() }, [])

  const handleRegister = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!newAddress.trim()) return
    setRegistering(true)
    setRegisterError(null)
    try {
      await api.registerGraderPlugin(newAddress.trim())
      setNewAddress('')
      await load()
    } catch (err) {
      setRegisterError(err instanceof ApiError ? err.message : 'Registration failed')
    } finally {
      setRegistering(false)
    }
  }

  const handleUpdate = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!editingTypeId || !editAddress.trim()) return
    setUpdating(true)
    setUpdateError(null)
    try {
      await api.updateGraderPlugin(editingTypeId, editAddress.trim())
      setEditingTypeId(null)
      setEditAddress('')
      await load()
    } catch (err) {
      setUpdateError(err instanceof ApiError ? err.message : 'Update failed')
    } finally {
      setUpdating(false)
    }
  }

  const handleDelete = async (typeId: string) => {
    if (!confirm(`Deregister grader plugin "${typeId}"?`)) return
    try {
      await api.deleteGraderPlugin(typeId)
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Deregistration failed')
    }
  }

  return (
    <div className="min-h-screen bg-gray-950 text-gray-100">
      <header className="border-b border-gray-800 px-6 py-4 flex items-center gap-4">
        <Link to="/" className="text-sm text-gray-400 hover:text-gray-200">← Workflows</Link>
        <h1 className="text-lg font-semibold">Grader Plugin Admin</h1>
      </header>

      <main className="max-w-3xl mx-auto px-6 py-8 space-y-8">

        {/* Register new plugin */}
        <section className="bg-gray-900 rounded-lg border border-gray-800 p-5">
          <h2 className="text-sm font-semibold text-gray-200 mb-4">Register New Grader Plugin</h2>
          <form onSubmit={handleRegister} className="flex gap-3">
            <input
              className="flex-1 rounded-md bg-gray-800 border border-gray-700 px-3 py-2 text-sm text-gray-100 placeholder-gray-500 focus:outline-none focus:border-indigo-500"
              placeholder="host:port (e.g. localhost:9001)"
              value={newAddress}
              onChange={e => setNewAddress(e.target.value)}
              disabled={registering}
            />
            <button
              type="submit"
              disabled={registering || !newAddress.trim()}
              className="px-4 py-2 text-sm font-medium rounded-md bg-indigo-600 hover:bg-indigo-500 disabled:opacity-50 transition-colors"
            >
              {registering ? 'Registering…' : 'Register'}
            </button>
          </form>
          {registerError && (
            <p className="mt-2 text-xs text-red-400">{registerError}</p>
          )}
        </section>

        {/* Plugin list */}
        <section>
          <h2 className="text-sm font-semibold text-gray-400 uppercase tracking-wider mb-3">
            Registered Grader Plugins
          </h2>

          {loading && <p className="text-sm text-gray-500">Loading…</p>}
          {error && <p className="text-sm text-red-400">{error}</p>}
          {!loading && !error && plugins.length === 0 && (
            <p className="text-sm text-gray-500">No grader plugins registered.</p>
          )}

          <div className="space-y-3">
            {plugins.map(p => (
              <div key={p.type_id} className="bg-gray-900 rounded-lg border border-gray-800 p-4">
                {editingTypeId === p.type_id ? (
                  <form onSubmit={handleUpdate} className="space-y-3">
                    <p className="text-xs font-mono text-gray-400">{p.type_id}</p>
                    <div className="flex gap-3">
                      <input
                        className="flex-1 rounded-md bg-gray-800 border border-gray-700 px-3 py-1.5 text-sm text-gray-100 placeholder-gray-500 focus:outline-none focus:border-indigo-500"
                        placeholder="new host:port"
                        value={editAddress}
                        onChange={e => setEditAddress(e.target.value)}
                        disabled={updating}
                        autoFocus
                      />
                      <button
                        type="submit"
                        disabled={updating || !editAddress.trim()}
                        className="px-3 py-1.5 text-sm font-medium rounded-md bg-indigo-600 hover:bg-indigo-500 disabled:opacity-50 transition-colors"
                      >
                        {updating ? 'Saving…' : 'Save'}
                      </button>
                      <button
                        type="button"
                        onClick={() => { setEditingTypeId(null); setUpdateError(null) }}
                        className="px-3 py-1.5 text-sm text-gray-400 hover:text-gray-200 transition-colors"
                      >
                        Cancel
                      </button>
                    </div>
                    {updateError && <p className="text-xs text-red-400">{updateError}</p>}
                  </form>
                ) : (
                  <div className="flex items-start justify-between gap-4">
                    <div className="min-w-0">
                      <p className="text-sm font-medium text-gray-100">{p.display_name}</p>
                      <p className="text-xs font-mono text-gray-400 mt-0.5">{p.type_id}</p>
                      <p className="text-xs text-gray-500 mt-0.5">{p.address}</p>
                      {p.description && (
                        <p className="text-xs text-gray-400 mt-1">{p.description}</p>
                      )}
                    </div>
                    <div className="flex gap-2 shrink-0">
                      <button
                        onClick={() => { setEditingTypeId(p.type_id); setEditAddress(p.address) }}
                        className="text-xs text-indigo-400 hover:text-indigo-300 transition-colors"
                      >
                        Update
                      </button>
                      <button
                        onClick={() => handleDelete(p.type_id)}
                        className="text-xs text-red-400 hover:text-red-300 transition-colors"
                      >
                        Remove
                      </button>
                    </div>
                  </div>
                )}
              </div>
            ))}
          </div>
        </section>

      </main>
    </div>
  )
}
