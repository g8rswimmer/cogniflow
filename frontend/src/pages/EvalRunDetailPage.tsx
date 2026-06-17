import { useEffect, useState } from 'react'
import { useParams, useSearchParams, Link } from 'react-router-dom'
import { api } from '../hooks/useApi'
import type { EvalRun, EvalRunCompare, CompareChangeType } from '../api/types'
import { EvalRunResultsTable } from '../components/eval/EvalRunResultsTable'
import { EvalRunStatusBadge } from '../components/eval/EvalRunStatusBadge'
import { formatDuration } from '../utils/formatDuration'

export function EvalRunDetailPage() {
  const { run_id: runId } = useParams<{ run_id: string }>()
  const [searchParams, setSearchParams] = useSearchParams()
  const baselineRunId = searchParams.get('baseline_run_id') ?? ''

  const [run, setRun] = useState<EvalRun | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [pollTick, setPollTick] = useState(0)

  const [completedSiblings, setCompletedSiblings] = useState<EvalRun[]>([])
  const [compareData, setCompareData] = useState<EvalRunCompare | null>(null)
  const [compareLoading, setCompareLoading] = useState(false)
  const [compareError, setCompareError] = useState<string | null>(null)

  useEffect(() => {
    if (!runId) return
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setLoading(true)
    api.getEvalRun(runId)
      .then(r => setRun(r))
      .catch(err => setError(err instanceof Error ? err.message : 'Failed to load run'))
      .finally(() => setLoading(false))
  }, [runId])

  // Poll while running — increment pollTick on error so the effect re-triggers
  // even when run hasn't changed, keeping the polling chain alive.
  useEffect(() => {
    if (!run || run.status === 'completed' || run.status === 'failed') return
    let alive = true
    const id = setTimeout(() => {
      if (!runId) return
      api.getEvalRun(runId)
        .then(r => { if (alive) setRun(r) })
        .catch(() => { if (alive) setPollTick(t => t + 1) })
    }, 2000)
    return () => { alive = false; clearTimeout(id) }
  }, [run, runId, pollTick])

  // Load sibling completed runs for the baseline selector once the suite is known.
  useEffect(() => {
    if (!run?.suite_id || !runId) return
    api.listEvalRuns(run.suite_id)
      .then(resp => {
        const siblings = (resp.eval_runs ?? []).filter(
          r => r.status === 'completed' && r.id !== runId
        )
        setCompletedSiblings(siblings)
      })
      .catch(() => { /* non-critical: selector simply stays empty */ })
  }, [run?.suite_id, runId])

  // Fetch comparison data when a baseline is selected and the head run is done.
  useEffect(() => {
    if (!runId || !baselineRunId) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setCompareData(null)
      return
    }
    const isTerminalNow = run?.status === 'completed' || run?.status === 'failed'
    if (!isTerminalNow) return

    setCompareLoading(true)
    setCompareData(null)
    setCompareError(null)
    api.compareEvalRuns(runId, baselineRunId)
      .then(d => setCompareData(d))
      .catch(err => setCompareError(err instanceof Error ? err.message : 'Compare failed'))
      .finally(() => setCompareLoading(false))
  }, [runId, baselineRunId, run?.status])

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
  const duration = formatDuration(run.started_at, run.finished_at)

  const compareMap: Map<string, CompareChangeType> | undefined = compareData
    ? new Map(compareData.cases.map(c => [c.test_case_id, c.change_type]))
    : undefined

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
          <EvalRunStatusBadge status={run.status} />
          {!isTerminal && (
            <span className="text-xs text-amber-400 animate-pulse">Polling every 2s…</span>
          )}
        </div>

        {/* Baseline selector */}
        {isTerminal && completedSiblings.length > 0 && (
          <div className="flex items-center gap-2 mb-4">
            <span className="text-xs text-gray-500">Compare to:</span>
            <select
              value={baselineRunId}
              onChange={e => {
                const val = e.target.value
                if (val) {
                  setSearchParams({ baseline_run_id: val })
                } else {
                  setSearchParams({})
                }
              }}
              className="text-xs bg-gray-800 border border-gray-600 rounded px-2 py-1 text-gray-300 focus:outline-none focus:border-gray-400"
            >
              <option value="">— no baseline —</option>
              {completedSiblings.map(r => (
                <option key={r.id} value={r.id}>
                  {r.id.slice(0, 8)}… ({new Date(r.created_at).toLocaleDateString()})
                </option>
              ))}
            </select>
            {baselineRunId && (
              <button
                type="button"
                onClick={() => setSearchParams({})}
                className="text-xs text-gray-500 hover:text-gray-300 transition-colors"
              >
                Clear
              </button>
            )}
          </div>
        )}

        {/* Summary */}
        <div className="rounded-lg bg-gray-800 border border-gray-700 px-5 py-4 mb-4">
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

        {/* Delta stats banner */}
        {compareLoading && (
          <div className="rounded-lg bg-gray-800 border border-gray-700 px-5 py-3 mb-4">
            <p className="text-xs text-gray-500">Loading comparison…</p>
          </div>
        )}
        {compareError && (
          <div className="rounded-lg bg-red-900/20 border border-red-800 px-5 py-3 mb-4">
            <p className="text-xs text-red-400">Comparison failed: {compareError}</p>
          </div>
        )}
        {compareData && !compareLoading && (
          <div className="rounded-lg bg-gray-800 border border-gray-700 px-5 py-3 mb-4 flex items-center gap-6 text-xs flex-wrap">
            <span className="text-gray-400 font-semibold">vs baseline</span>
            {compareData.regressed_count > 0 && (
              <span className="text-red-400 font-semibold">
                -{compareData.regressed_count} regressed
              </span>
            )}
            {compareData.improved_count > 0 && (
              <span className="text-green-400 font-semibold">
                +{compareData.improved_count} improved
              </span>
            )}
            <span className="text-gray-500">
              {compareData.unchanged_count} unchanged
            </span>
            {compareData.new_case_count > 0 && (
              <span className="text-indigo-400">{compareData.new_case_count} new</span>
            )}
            {compareData.missing_count > 0 && (
              <span className="text-amber-400">{compareData.missing_count} missing</span>
            )}
          </div>
        )}

        {/* Test case results */}
        {results.length > 0 ? (
          <EvalRunResultsTable results={results} compareMap={compareMap} />
        ) : !isTerminal ? (
          <p className="text-center text-gray-500 text-sm py-8">
            Test cases are running… results will appear here.
          </p>
        ) : null}
      </div>
    </div>
  )
}
