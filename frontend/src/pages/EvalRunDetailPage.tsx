import { useEffect, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { api } from '../hooks/useApi'
import type { EvalRun } from '../api/types'
import { EvalRunResultsTable } from '../components/eval/EvalRunResultsTable'

function StatusBadge({ status }: { status: string }) {
  const colors: Record<string, string> = {
    pending: 'bg-gray-600 text-gray-300',
    running: 'bg-amber-700 text-amber-200',
    completed: 'bg-green-700 text-green-200',
    failed: 'bg-red-700 text-red-200',
  }
  return (
    <span className={`px-2 py-0.5 rounded text-xs font-semibold ${colors[status] ?? 'bg-gray-600 text-gray-300'}`}>
      {status}
    </span>
  )
}

export function EvalRunDetailPage() {
  const { run_id: runId } = useParams<{ run_id: string }>()
  const [run, setRun] = useState<EvalRun | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!runId) return
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setLoading(true)
    api.getEvalRun(runId)
      .then(r => setRun(r))
      .catch(err => setError(err instanceof Error ? err.message : 'Failed to load run'))
      .finally(() => setLoading(false))
  }, [runId])

  // Poll while running
  useEffect(() => {
    if (!run || run.status === 'completed' || run.status === 'failed') return
    let alive = true
    const id = setTimeout(() => {
      if (!runId) return
      api.getEvalRun(runId)
        .then(r => { if (alive) setRun(r) })
        .catch(() => undefined)
    }, 2000)
    return () => { alive = false; clearTimeout(id) }
  }, [run, runId])

  if (loading) {
    return (
      <div className="min-h-screen bg-gray-950 flex items-center justify-center">
        <p className="text-gray-400 text-sm">Loading run…</p>
      </div>
    )
  }

  if (error || !run) {
    return (
      <div className="min-h-screen bg-gray-950 flex items-center justify-center">
        <p className="text-red-400">{error ?? 'Run not found'}</p>
      </div>
    )
  }

  const isTerminal = run.status === 'completed' || run.status === 'failed'
  const results = run.test_case_results ?? []

  const duration = (() => {
    if (!run.started_at || !run.finished_at) return null
    const ms = new Date(run.finished_at).getTime() - new Date(run.started_at).getTime()
    const s = Math.round(ms / 1000)
    return s < 60 ? `${s}s` : `${Math.floor(s / 60)}m ${s % 60}s`
  })()

  return (
    <div className="min-h-screen bg-gray-950 text-gray-100">
      <div className="max-w-4xl mx-auto px-4 py-8">
        {/* Header */}
        <div className="flex items-center gap-4 mb-6">
          <Link
            to={`/eval-suites/${run.suite_id}`}
            className="text-indigo-400 hover:text-indigo-300 text-sm transition-colors"
          >
            ← Back to Suite
          </Link>
          <h1 className="text-xl font-bold text-gray-100">Eval Run</h1>
          <StatusBadge status={run.status} />
          {!isTerminal && (
            <span className="text-xs text-amber-400 animate-pulse">Polling every 2s…</span>
          )}
        </div>

        {/* Summary */}
        <div className="rounded-lg bg-gray-800 border border-gray-700 px-5 py-4 mb-6">
          <div className="grid grid-cols-4 gap-4 text-center">
            <div>
              <div className="text-2xl font-bold text-gray-100">{run.total_cases}</div>
              <div className="text-xs text-gray-500 mt-0.5">Total</div>
            </div>
            <div>
              <div className="text-2xl font-bold text-green-400">{run.passed_count}</div>
              <div className="text-xs text-gray-500 mt-0.5">Passed</div>
            </div>
            <div>
              <div className="text-2xl font-bold text-red-400">{run.failed_count}</div>
              <div className="text-xs text-gray-500 mt-0.5">Failed</div>
            </div>
            <div>
              <div className="text-2xl font-bold text-amber-400">{run.error_count}</div>
              <div className="text-xs text-gray-500 mt-0.5">Errors</div>
            </div>
          </div>
          {run.started_at && (
            <p className="text-xs text-gray-600 text-center mt-3">
              Started {new Date(run.started_at).toLocaleString()}
              {duration && <> · {duration}</>}
            </p>
          )}
        </div>

        {/* Test case results */}
        {results.length > 0 ? (
          <EvalRunResultsTable results={results} />
        ) : !isTerminal ? (
          <p className="text-center text-gray-500 text-sm py-8">
            Test cases are running… results will appear here.
          </p>
        ) : null}
      </div>
    </div>
  )
}
