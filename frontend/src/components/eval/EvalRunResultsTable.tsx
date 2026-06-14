import { useState } from 'react'
import { Link } from 'react-router-dom'
import type { TestCaseResult } from '../../api/types'
import { GraderResultRow } from './GraderResultRow'

function WorkflowRunStatusChip({ status }: { status: string }) {
  const colors: Record<string, string> = {
    succeeded: 'bg-green-900/40 text-green-300',
    failed: 'bg-red-900/40 text-red-300',
    pending: 'bg-gray-700 text-gray-400',
    running: 'bg-amber-900/40 text-amber-300',
  }
  return (
    <span className={`px-1.5 py-0.5 rounded text-[10px] font-semibold ${colors[status] ?? 'bg-gray-700 text-gray-400'}`}>
      {status}
    </span>
  )
}

function TestCaseResultRow({ result }: { result: TestCaseResult }) {
  const [expanded, setExpanded] = useState(false)
  const workflowFailed = result.workflow_run_status === 'failed'

  return (
    <div
      className={`rounded-lg border ${
        result.passed
          ? 'border-green-800 bg-green-900/10'
          : 'border-red-800 bg-red-900/10'
      }`}
    >
      {/* Summary row — click to expand */}
      <button
        type="button"
        onClick={() => setExpanded(e => !e)}
        className="w-full flex items-center gap-3 px-4 py-3 text-left"
      >
        <span className="text-xs flex-shrink-0 text-gray-500 w-3">
          {expanded ? '▾' : '▸'}
        </span>
        <span className="flex-1 text-sm font-medium text-gray-100 truncate">
          {result.test_case_name}
        </span>
        <div className="flex items-center gap-2 flex-shrink-0">
          <WorkflowRunStatusChip status={result.workflow_run_status} />
          <Link
            to={`/runs/${result.workflow_run_id}`}
            className="text-xs text-indigo-400 hover:text-indigo-300 transition-colors"
            onClick={e => e.stopPropagation()}
          >
            View Run →
          </Link>
          <span
            className={`text-xs font-semibold ${result.passed ? 'text-green-400' : 'text-red-400'}`}
          >
            {result.passed ? '✓ passed' : '✗ failed'}
          </span>
        </div>
      </button>

      {/* Expanded grader details */}
      {expanded && (
        <div className="border-t border-gray-700/40 px-4 py-3 space-y-3">
          {workflowFailed && (
            <div className="rounded bg-red-900/30 border border-red-800 px-3 py-2">
              <p className="text-xs text-red-300 font-semibold">
                Workflow run failed — graders that depended on node output could not be evaluated.
              </p>
            </div>
          )}
          {result.grader_results.length === 0 ? (
            <p className="text-xs text-gray-600 italic">
              No graders — test case {result.passed ? 'passed' : 'failed'} based on workflow completion.
            </p>
          ) : (
            <div className="space-y-3">
              {result.grader_results.map((gr, i) => (
                <GraderResultRow key={i} result={gr} />
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  )
}

interface Props {
  results: TestCaseResult[]
}

export function EvalRunResultsTable({ results }: Props) {
  return (
    <div className="space-y-2">
      {results.map(tcr => (
        <TestCaseResultRow key={tcr.id} result={tcr} />
      ))}
    </div>
  )
}
