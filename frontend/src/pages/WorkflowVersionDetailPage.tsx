import { useEffect, useState } from 'react'
import { useParams, Link, useNavigate } from 'react-router-dom'
import { api } from '../hooks/useApi'
import type { WorkflowVersion } from '../api/types'

export function WorkflowVersionDetailPage() {
  const { id, version_number } = useParams<{ id: string; version_number: string }>()
  const navigate = useNavigate()
  const [version, setVersion] = useState<WorkflowVersion | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [restoring, setRestoring] = useState(false)
  const [confirm, setConfirm] = useState(false)

  const versionNum = Number(version_number)

  useEffect(() => {
    if (!id || !versionNum) return
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setLoading(true)
    api.getWorkflowVersion(id, versionNum)
      .then(r => setVersion(r.version))
      .catch(err => setError(err instanceof Error ? err.message : 'Failed to load version'))
      .finally(() => setLoading(false))
  }, [id, versionNum])

  async function handleRestore() {
    if (!id || !versionNum) return
    setRestoring(true)
    try {
      await api.restoreWorkflowVersion(id, versionNum)
      navigate(`/workflows/${id}`)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Restore failed')
      setRestoring(false)
      setConfirm(false)
    }
  }

  const def = version?.definition

  return (
    <div className="min-h-screen bg-gray-950 text-gray-100">
      <div className="max-w-4xl mx-auto px-4 py-8">
        {/* Header */}
        <div className="flex items-center justify-between mb-6">
          <div className="flex items-center gap-4">
            <Link
              to={`/workflows/${id}/versions`}
              className="text-indigo-400 hover:text-indigo-300 text-sm transition-colors"
            >
              ← Back to History
            </Link>
            <h1 className="text-xl font-bold text-gray-100">
              Version #{versionNum}
            </h1>
          </div>

          {!confirm ? (
            <button
              onClick={() => setConfirm(true)}
              disabled={loading || !!error}
              className="rounded-md bg-indigo-600 hover:bg-indigo-500 disabled:opacity-40 disabled:cursor-not-allowed px-3 py-1.5 text-sm font-medium text-white transition-colors"
            >
              Restore this version
            </button>
          ) : (
            <div className="flex items-center gap-2">
              <span className="text-sm text-yellow-400">Confirm restore?</span>
              <button
                onClick={handleRestore}
                disabled={restoring}
                className="rounded-md bg-yellow-600 hover:bg-yellow-500 disabled:opacity-40 px-3 py-1.5 text-sm font-medium text-white transition-colors"
              >
                {restoring ? 'Restoring…' : 'Yes, restore'}
              </button>
              <button
                onClick={() => setConfirm(false)}
                disabled={restoring}
                className="rounded-md border border-gray-600 bg-gray-700 hover:bg-gray-600 px-3 py-1.5 text-sm font-medium text-gray-200 transition-colors"
              >
                Cancel
              </button>
            </div>
          )}
        </div>

        {loading && <p className="text-gray-400 text-sm">Loading version…</p>}
        {error && <p className="text-red-400 text-sm">{error}</p>}

        {version && def && (
          <>
            {/* Metadata */}
            <div className="rounded-lg bg-gray-800 border border-gray-700 px-4 py-3 mb-4 text-sm space-y-1">
              <div>
                <span className="text-gray-400">Saved at: </span>
                <span className="text-gray-200">{new Date(version.created_at).toLocaleString()}</span>
              </div>
              <div>
                <span className="text-gray-400">Workflow name: </span>
                <span className="text-gray-200">{def.name}</span>
              </div>
              <div>
                <span className="text-gray-400">Trigger: </span>
                <span className="text-gray-200">{def.trigger.kind}</span>
                {def.trigger.cron_expr && (
                  <span className="text-gray-400 ml-1">({def.trigger.cron_expr})</span>
                )}
              </div>
              <div>
                <span className="text-gray-400">Nodes: </span>
                <span className="text-gray-200">{def.nodes.length}</span>
                <span className="text-gray-400 ml-2">Edges: </span>
                <span className="text-gray-200">{def.edges.length}</span>
              </div>
            </div>

            {/* Note about sensitive values */}
            {def.nodes.some(n => Object.values(n.config ?? {}).includes('***')) && (
              <div className="rounded-md border border-yellow-700 bg-yellow-900/30 px-3 py-2 mb-4 text-xs text-yellow-300">
                Sensitive config values (API keys, passwords) are shown as *** for security. They will be restored correctly when you restore this version.
              </div>
            )}

            {/* Node list */}
            <h2 className="text-sm font-semibold text-gray-400 uppercase tracking-wider mb-2">Nodes</h2>
            {def.nodes.length === 0 ? (
              <p className="text-gray-500 italic text-sm">No nodes in this version.</p>
            ) : (
              <div className="space-y-2">
                {def.nodes.map(node => (
                  <div key={node.id} className="rounded-lg bg-gray-800 border border-gray-700 px-4 py-3">
                    <div className="flex items-center justify-between">
                      <span className="text-sm font-medium text-gray-200">{node.label || node.id}</span>
                      <span className="text-xs font-mono text-gray-500">{node.type_id}</span>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </>
        )}
      </div>
    </div>
  )
}
