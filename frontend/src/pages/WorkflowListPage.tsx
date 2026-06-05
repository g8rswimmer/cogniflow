import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../hooks/useApi'
import type { Workflow } from '../api/types'

function formatDate(iso: string) {
  try {
    return new Intl.DateTimeFormat(undefined, {
      dateStyle: 'medium',
      timeStyle: 'short',
    }).format(new Date(iso))
  } catch {
    return iso
  }
}

export function WorkflowListPage() {
  const [workflows, setWorkflows] = useState<Workflow[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [deleting, setDeleting] = useState<string | null>(null)

  const load = async () => {
    setLoading(true)
    setError(null)
    try {
      const { workflows: wfs } = await api.listWorkflows()
      setWorkflows(wfs ?? [])
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load workflows')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { load() }, [])

  const handleDelete = async (id: string, name: string) => {
    if (!confirm(`Delete workflow "${name}"?`)) return
    setDeleting(id)
    try {
      await api.deleteWorkflow(id)
      setWorkflows(ws => ws.filter(w => w.id !== id))
    } catch (err) {
      alert(err instanceof Error ? err.message : 'Delete failed')
    } finally {
      setDeleting(null)
    }
  }

  return (
    <div className="min-h-screen bg-gray-950 text-gray-100">
      {/* Top bar */}
      <header className="bg-gray-900 border-b border-gray-700 px-6 py-3 flex items-center justify-between">
        <h1 className="text-lg font-semibold text-gray-100">cogniflow</h1>
        <Link
          to="/workflows/new"
          className="rounded-lg bg-indigo-600 hover:bg-indigo-500 text-white text-sm font-medium px-4 py-2 transition-colors"
        >
          + New Workflow
        </Link>
      </header>

      <main className="max-w-4xl mx-auto px-6 py-8">
        <h2 className="text-xl font-semibold text-gray-100 mb-6">Workflows</h2>

        {loading && (
          <div className="text-gray-400 text-sm">Loading…</div>
        )}

        {error && (
          <div className="rounded-lg border border-red-700 bg-red-900/20 px-4 py-3 text-sm text-red-300">
            {error}
          </div>
        )}

        {!loading && !error && workflows.length === 0 && (
          <div className="text-center py-16">
            <div className="text-4xl mb-3">⚙️</div>
            <p className="text-gray-400 mb-4">No workflows yet.</p>
            <Link
              to="/workflows/new"
              className="rounded-lg bg-indigo-600 hover:bg-indigo-500 text-white text-sm font-medium px-4 py-2 transition-colors"
            >
              Create your first workflow
            </Link>
          </div>
        )}

        {workflows.length > 0 && (
          <div className="divide-y divide-gray-800 rounded-xl border border-gray-700 overflow-hidden">
            {workflows.map(wf => (
              <div
                key={wf.id}
                className="flex items-center gap-4 px-5 py-4 bg-gray-900 hover:bg-gray-800 transition-colors"
              >
                <div className="flex-1 min-w-0">
                  <Link
                    to={`/workflows/${wf.id}`}
                    className="text-sm font-medium text-gray-100 hover:text-indigo-300 transition-colors truncate block"
                  >
                    {wf.name}
                  </Link>
                  <div className="text-xs text-gray-500 mt-0.5 flex gap-3">
                    <span className="capitalize">{wf.trigger?.kind ?? 'manual'}</span>
                    <span>{(wf.nodes ?? []).length} node{(wf.nodes ?? []).length !== 1 ? 's' : ''}</span>
                    <span>{formatDate(wf.updated_at)}</span>
                  </div>
                </div>

                <div className="flex gap-2 flex-shrink-0">
                  <Link
                    to={`/workflows/${wf.id}`}
                    className="text-xs rounded-md border border-gray-600 bg-gray-700 hover:bg-gray-600 text-gray-200 px-2.5 py-1.5 transition-colors"
                  >
                    Edit
                  </Link>
                  <Link
                    to={`/workflows/${wf.id}/runs`}
                    className="text-xs rounded-md border border-gray-600 bg-gray-700 hover:bg-gray-600 text-gray-200 px-2.5 py-1.5 transition-colors"
                  >
                    Runs
                  </Link>
                  <button
                    onClick={() => handleDelete(wf.id, wf.name)}
                    disabled={deleting === wf.id}
                    className="text-xs rounded-md border border-gray-700 text-gray-500 hover:border-red-700 hover:text-red-400 px-2.5 py-1.5 transition-colors disabled:opacity-50"
                  >
                    {deleting === wf.id ? '…' : 'Delete'}
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </main>
    </div>
  )
}
