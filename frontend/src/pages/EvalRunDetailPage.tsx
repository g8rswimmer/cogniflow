import { useEffect, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { api } from '../hooks/useApi'
import type { EvalRun } from '../api/types'

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

  return (
    <div className="min-h-screen bg-gray-950 text-gray-100">
      <div className="max-w-4xl mx-auto px-4 py-8">
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
              {run.finished_at && (
                <> · {Math.round((new Date(run.finished_at).getTime() - new Date(run.started_at).getTime()) / 1000)}s</>
              )}
            </p>
          )}
        </div>

        {/* Test case results (ME5 will expand this) */}
        {run.test_case_results && run.test_case_results.length > 0 ? (
          <div className="space-y-2">
            {run.test_case_results.map(tcr => (
              <div
                key={tcr.id}
                className={`rounded-lg border px-4 py-3 ${
                  tcr.passed
                    ? 'border-green-800 bg-green-900/20'
                    : 'border-red-800 bg-red-900/20'
                }`}
              >
                <div className="flex items-center justify-between">
                  <span className="text-sm font-medium text-gray-100">{tcr.test_case_name}</span>
                  <div className="flex items-center gap-2">
                    <span className={`text-xs font-semibold ${tcr.passed ? 'text-green-400' : 'text-red-400'}`}>
                      {tcr.passed ? '✓ passed' : '✗ failed'}
                    </span>
                    <Link
                      to={`/runs/${tcr.workflow_run_id}`}
                      className="text-xs text-indigo-400 hover:text-indigo-300 transition-colors"
                      onClick={e => e.stopPropagation()}
                    >
                      View Run →
                    </Link>
                  </div>
                </div>
                {tcr.grader_results.length > 0 && (
                  <div className="mt-2 space-y-1">
                    {tcr.grader_results.map((gr, i) => (
                      <div key={i} className="flex items-start gap-2 text-xs text-gray-400">
                        <span className={`font-semibold flex-shrink-0 ${
                          gr.verdict === 'pass' ? 'text-green-400' :
                          gr.verdict === 'fail' ? 'text-red-400' : 'text-amber-400'
                        }`}>
                          {gr.verdict === 'pass' ? '✓' : gr.verdict === 'fail' ? '✗' : '!'}
                        </span>
                        <span className="text-gray-300">{gr.grader_name}</span>
                        {gr.explanation && (
                          <span className="text-gray-500 truncate">— {gr.explanation}</span>
                        )}
                      </div>
                    ))}
                  </div>
                )}
              </div>
            ))}
          </div>
        ) : (
          !isTerminal && (
            <p className="text-center text-gray-500 text-sm py-8">
              Test cases are running… results will appear here.
            </p>
          )
        )}
      </div>
    </div>
  )
}
