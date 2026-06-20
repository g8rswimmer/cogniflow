import { useEffect, useState } from 'react'
import { useParams, useSearchParams, Link } from 'react-router-dom'
import { api } from '../hooks/useApi'
import type { EvalRun, EvalRunCompare, CompareChangeType, TestCaseResult } from '../api/types'
import { EvalRunResultsTable } from '../components/eval/EvalRunResultsTable'
import { EvalRunStatusBadge } from '../components/eval/EvalRunStatusBadge'
import { formatDuration } from '../utils/formatDuration'
import { useEvalRunEvents } from '../hooks/useEvalRunEvents'

export function EvalRunDetailPage() {
  const { run_id: runId } = useParams<{ run_id: string }>()
  const [searchParams, setSearchParams] = useSearchParams()
  const baselineRunId = searchParams.get('baseline_run_id') ?? ''

  const [run, setRun] = useState<EvalRun | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const [completedSiblings, setCompletedSiblings] = useState<EvalRun[]>([])
  const [compareData, setCompareData] = useState<EvalRunCompare | null>(null)
  const [compareLoading, setCompareLoading] = useState(false)
  const [compareError, setCompareError] = useState<string | null>(null)

  // WebSocket streaming — always connects; fast-path for terminal runs.
  const { liveResults, summary: wsSummary, isTerminal: wsTerminal, isConnected, connectionLost } = useEvalRunEvents(runId ?? null)

  // Initial fetch.
  useEffect(() => {
    if (!runId) return
    setLoading(true)
    api.getEvalRun(runId)
      .then(r => setRun(r))
      .catch(err => setError(err instanceof Error ? err.message : 'Failed to load run'))
      .finally(() => setLoading(false))
  }, [runId])

  // When the WebSocket declares the run terminal, do a final REST fetch to get
  // the authoritative summary counts and updated status from the DB.
  useEffect(() => {
    if (!wsTerminal || !runId) return
    api.getEvalRun(runId)
      .then(r => setRun(r))
      .catch(() => { /* non-critical: UI already has WS summary */ })
  }, [wsTerminal, runId])

  // Fallback poll: if the WS closed before a terminal event was received (e.g.
  // a race where the run completed just before the server subscribed the client),
  // poll every 2 s until the run reaches a terminal state. This mirrors the old
  // polling behaviour and ensures the page never stays stuck.
  useEffect(() => {
    if (!connectionLost || !runId || wsTerminal) return
    const isAlreadyTerminal = run?.status === 'completed' || run?.status === 'failed'
    if (isAlreadyTerminal) return

    let alive = true
    const id = setTimeout(() => {
      if (!alive || !runId) return
      api.getEvalRun(runId)
        .then(r => { if (alive) setRun(r) })
        .catch(() => {})
    }, 2000)
    return () => { alive = false; clearTimeout(id) }
  }, [connectionLost, run, runId, wsTerminal])

  // Load sibling completed runs for the baseline selector.
  // Re-fires when run.status changes so a newly completed run appears in the
  // selector without a full page reload.
  useEffect(() => {
    if (!run?.suite_id || !runId) return
    api.listEvalRuns(run.suite_id, { limit: 200 })
      .then(resp => {
        const siblings = (resp.eval_runs ?? []).filter(
          r => r.status === 'completed' && r.id !== runId
        )
        setCompletedSiblings(siblings)
      })
      .catch(() => { /* non-critical: selector simply stays empty */ })
  }, [run?.suite_id, runId, run?.status])

  // Fetch comparison data when a baseline is selected and the head run is completed.
  useEffect(() => {
    if (!runId || !baselineRunId) {
      setCompareData(null)
      return
    }
    if (run?.status !== 'completed') return

    let alive = true
    setCompareLoading(true)
    setCompareData(null)
    setCompareError(null)
    api.compareEvalRuns(runId, baselineRunId)
      .then(d => { if (alive) setCompareData(d) })
      .catch(err => { if (alive) setCompareError(err instanceof Error ? err.message : 'Compare failed') })
      .finally(() => { if (alive) setCompareLoading(false) })
    return () => { alive = false }
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

  // Prefer live-streamed results; fall back to whatever the REST fetch returned.
  const displayResults: TestCaseResult[] = liveResults.length > 0
    ? liveResults
    : (run.test_case_results ?? [])

  // While streaming, compute live counts from received results so the summary
  // updates progressively rather than staying at zero until terminal.
  const livePassedCount = liveResults.filter(r => r.passed).length
  const liveErrorCount = liveResults.filter(r => r.workflow_run_status === 'failed').length
  const liveFailedCount = liveResults.length - livePassedCount - liveErrorCount

  const displayTotalCases = run.total_cases
  const displayPassedCount = isTerminal ? (wsSummary?.passed_count ?? run.passed_count) : livePassedCount
  const displayFailedCount = isTerminal ? (wsSummary?.failed_count ?? run.failed_count) : liveFailedCount
  const displayErrorCount  = isTerminal ? (wsSummary?.error_count  ?? run.error_count)  : liveErrorCount

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
          {!isTerminal && isConnected && (
            <span className="text-xs text-green-400 animate-pulse">
              Live · {liveResults.length}/{displayTotalCases} complete
            </span>
          )}
          {!isTerminal && !isConnected && (
            <span className="text-xs text-amber-400">Connecting…</span>
          )}
        </div>

        {/* Baseline selector — only shown for completed runs with available siblings */}
        {run.status === 'completed' && completedSiblings.length > 0 && (
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
              <div className="text-2xl font-bold text-gray-100">{displayTotalCases}</div>
              <div className="text-xs text-gray-500 mt-0.5">Total</div>
            </div>
            <div>
              <div className="text-2xl font-bold text-green-400">{displayPassedCount}</div>
              <div className="text-xs text-gray-500 mt-0.5">Passed</div>
            </div>
            <div>
              <div className="text-2xl font-bold text-red-400">{displayFailedCount}</div>
              <div className="text-xs text-gray-500 mt-0.5">Failed</div>
            </div>
            <div>
              <div className="text-2xl font-bold text-amber-400">{displayErrorCount}</div>
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

        {/* Test case results — appear progressively during streaming */}
        {displayResults.length > 0 ? (
          <EvalRunResultsTable results={displayResults} compareMap={compareMap} />
        ) : !isTerminal ? (
          <p className="text-center text-gray-500 text-sm py-8">
            Waiting for first test case to complete…
          </p>
        ) : null}
      </div>
    </div>
  )
}
