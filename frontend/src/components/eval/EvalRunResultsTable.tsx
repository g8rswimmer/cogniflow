import { useState } from 'react'
import { Link } from 'react-router-dom'
import type { TestCaseResult, CompareChangeType } from '../../api/types'
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

const CHANGE_BADGE: Record<CompareChangeType, { label: string; className: string }> = {
  regressed: { label: 'regressed', className: 'bg-red-900/50 text-red-300 border border-red-800' },
  improved:  { label: 'improved',  className: 'bg-green-900/50 text-green-300 border border-green-800' },
  unchanged: { label: 'unchanged', className: 'bg-gray-700 text-gray-400' },
  new_case:  { label: 'new',       className: 'bg-indigo-900/50 text-indigo-300' },
  missing:   { label: 'missing',   className: 'bg-amber-900/40 text-amber-400' },
}

function TestCaseResultRow({ result, changeType }: { result: TestCaseResult; changeType?: CompareChangeType }) {
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
          {changeType !== undefined && (
            <span className={`px-1.5 py-0.5 rounded text-[10px] font-semibold ${CHANGE_BADGE[changeType].className}`}>
              {CHANGE_BADGE[changeType].label}
            </span>
          )}
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
              {result.grader_results.map(gr => (
                <GraderResultRow key={gr.grader_id} result={gr} />
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
  compareMap?: Map<string, CompareChangeType>
}

export function EvalRunResultsTable({ results, compareMap }: Props) {
  return (
    <div className="space-y-2">
      {results.map(tcr => (
        <TestCaseResultRow
          key={tcr.id}
          result={tcr}
          changeType={compareMap?.get(tcr.test_case_id)}
        />
      ))}
    </div>
  )
}
