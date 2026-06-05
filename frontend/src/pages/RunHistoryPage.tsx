import { useEffect, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { api } from '../hooks/useApi'
import type { Run, RunStatus } from '../api/types'

const statusColors: Record<RunStatus, string> = {
  pending: 'bg-gray-600 text-gray-300',
  running: 'bg-amber-700 text-amber-200',
  succeeded: 'bg-green-700 text-green-200',
  failed: 'bg-red-700 text-red-200',
}

function StatusBadge({ status }: { status: RunStatus }) {
  return (
    <span className={`px-2 py-0.5 rounded text-xs font-semibold ${statusColors[status] ?? 'bg-gray-600 text-gray-300'}`}>
      {status}
    </span>
  )
}

export function RunHistoryPage() {
  const { id } = useParams<{ id: string }>()
  const [runs, setRuns] = useState<Run[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!id) return
    setLoading(true)
    api.listRuns(id)
      .then(r => setRuns(r.runs ?? []))
      .catch(err => setError(err instanceof Error ? err.message : 'Failed to load runs'))
      .finally(() => setLoading(false))
  }, [id])

  return (
    <div className="min-h-screen bg-gray-950 text-gray-100">
      <div className="max-w-4xl mx-auto px-4 py-8">
        {/* Header */}
        <div className="flex items-center gap-4 mb-6">
          <Link
            to={`/workflows/${id}`}
            className="text-indigo-400 hover:text-indigo-300 text-sm transition-colors"
          >
            ← Back to Editor
          </Link>
          <h1 className="text-xl font-bold text-gray-100">Run History</h1>
        </div>

        {loading && (
          <p className="text-gray-400 text-sm">Loading runs…</p>
        )}

        {error && (
          <p className="text-red-400 text-sm">{error}</p>
        )}

        {!loading && !error && runs.length === 0 && (
          <p className="text-gray-500 italic text-sm">No runs yet. Trigger a run from the editor.</p>
        )}

        <div className="space-y-2">
          {runs.map(run => (
            <Link
              key={run.run_id}
              to={`/runs/${run.run_id}`}
              className="flex items-center justify-between rounded-lg bg-gray-800 border border-gray-700 px-4 py-3 hover:bg-gray-700 transition-colors group"
            >
              <div className="min-w-0">
                <div className="text-sm font-mono text-gray-300 truncate group-hover:text-gray-100 transition-colors">
                  {run.run_id}
                </div>
                <div className="text-xs text-gray-500 mt-0.5">
                  {new Date(run.started_at).toLocaleString()} · triggered by {run.triggered_by}
                  {run.finished_at && (
                    <> · {Math.round((new Date(run.finished_at).getTime() - new Date(run.started_at).getTime()) / 1000)}s</>
                  )}
                </div>
              </div>
              <StatusBadge status={run.status} />
            </Link>
          ))}
        </div>
      </div>
    </div>
  )
}
