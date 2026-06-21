import { useEffect, useState } from 'react'
import { useParams, Link, useNavigate } from 'react-router-dom'
import { api } from '../hooks/useApi'
import type { WorkflowVersionSummary } from '../api/types'

export function WorkflowVersionHistoryPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [versions, setVersions] = useState<WorkflowVersionSummary[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!id) return
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setLoading(true)
    api.listWorkflowVersions(id)
      .then(r => setVersions(r.versions ?? []))
      .catch(err => setError(err instanceof Error ? err.message : 'Failed to load versions'))
      .finally(() => setLoading(false))
  }, [id])

  return (
    <div className="min-h-screen bg-gray-950 text-gray-100">
      <div className="max-w-4xl mx-auto px-4 py-8">
        <div className="flex items-center gap-4 mb-6">
          <Link
            to={`/workflows/${id}`}
            className="text-indigo-400 hover:text-indigo-300 text-sm transition-colors"
          >
            ← Back to Editor
          </Link>
          <h1 className="text-xl font-bold text-gray-100">Version History</h1>
        </div>

        {loading && <p className="text-gray-400 text-sm">Loading versions…</p>}
        {error && <p className="text-red-400 text-sm">{error}</p>}

        {!loading && !error && versions.length === 0 && (
          <p className="text-gray-500 italic text-sm">
            No versions yet. Versions are created automatically each time you save the workflow.
          </p>
        )}

        <div className="space-y-2">
          {versions.map(ver => (
            <div
              key={ver.id}
              onClick={() => navigate(`/workflows/${id}/versions/${ver.version_number}`)}
              className="flex items-center justify-between rounded-lg bg-gray-800 border border-gray-700 px-4 py-3 hover:bg-gray-700 transition-colors cursor-pointer group"
            >
              <div className="min-w-0">
                <div className="text-sm font-semibold text-gray-200 group-hover:text-gray-100 transition-colors">
                  Version #{ver.version_number}
                </div>
                <div className="text-xs text-gray-500 mt-0.5">
                  {new Date(ver.created_at).toLocaleString()}
                  {' · '}
                  <span className="text-gray-400">{ver.node_count} node{ver.node_count !== 1 ? 's' : ''}</span>
                </div>
              </div>
              <span className="text-xs text-gray-500 group-hover:text-gray-400">View →</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}
