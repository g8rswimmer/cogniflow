import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../../hooks/useApi'
import type { EvalRun } from '../../api/types'
import { EvalRunStatusBadge } from './EvalRunStatusBadge'
import { formatDuration } from '../../utils/formatDuration'

interface Props {
  suiteId: string
}

export function EvalRunHistory({ suiteId }: Props) {
  const navigate = useNavigate()
  const [runs, setRuns] = useState<EvalRun[]>([])
  const [loading, setLoading] = useState(false)
  const [fetchError, setFetchError] = useState(false)
  const [open, setOpen] = useState(false)

  useEffect(() => {
    if (!open) return
    let alive = true
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setLoading(true)
    setFetchError(false)
    api.listEvalRuns(suiteId)
      .then(r => { if (alive) setRuns(r.eval_runs ?? []) })
      .catch(() => { if (alive) setFetchError(true) })
      .finally(() => { if (alive) setLoading(false) })
    return () => { alive = false }
  }, [suiteId, open])

  return (
    <div className="border border-gray-700 rounded-lg overflow-hidden">
      <button
        type="button"
        onClick={() => setOpen(o => !o)}
        className="w-full flex items-center justify-between px-4 py-2.5 bg-gray-800 hover:bg-gray-700 transition-colors text-left"
      >
        <span className="text-xs font-semibold text-gray-400 uppercase tracking-wide">Run History</span>
        <span className="text-gray-500 text-xs">{open ? '▾' : '▸'}</span>
      </button>

      {open && (
        <div className="border-t border-gray-700 bg-gray-900">
          {loading ? (
            <p className="text-xs text-gray-500 px-4 py-3">Loading…</p>
          ) : fetchError ? (
            <p className="text-xs text-red-400 px-4 py-3">Failed to load run history.</p>
          ) : runs.length === 0 ? (
            <p className="text-xs text-gray-600 italic px-4 py-3">No runs yet.</p>
          ) : (
            <div className="divide-y divide-gray-800">
              {runs.map(run => {
                const dur = formatDuration(run.started_at, run.finished_at)
                return (
                  <div
                    key={run.id}
                    className="flex items-center justify-between px-4 py-2.5 hover:bg-gray-800 transition-colors cursor-pointer"
                    onClick={() => navigate(`/eval-runs/${run.id}`)}
                  >
                    <div className="min-w-0">
                      <span className="text-xs font-mono text-gray-400">{run.id.slice(0, 8)}…</span>
                      <span className="text-xs text-gray-600 ml-2">
                        {run.started_at
                          ? new Date(run.started_at).toLocaleString()
                          : new Date(run.created_at).toLocaleString()}
                      </span>
                      {dur && (
                        <span className="text-xs text-gray-600 ml-2">{dur}</span>
                      )}
                    </div>
                    <div className="flex items-center gap-2 flex-shrink-0">
                      <span className="text-xs text-gray-400">
                        {run.passed_count}/{run.total_cases} passed
                      </span>
                      <EvalRunStatusBadge status={run.status} size="sm" />
                    </div>
                  </div>
                )
              })}
            </div>
          )}
        </div>
      )}
    </div>
  )
}
