import { useState } from 'react'
import type { GraderResult } from '../../api/types'
import { ChecklistResultDetail } from './ChecklistResultDetail'

const GRADER_LABEL: Record<string, string> = {
  string_match: 'String Match',
  numeric_threshold: 'Numeric',
  llm_judge: 'LLM Judge',
  json_schema: 'JSON Schema',
  checklist: 'Checklist',
}

const VERDICT_STYLE: Record<string, string> = {
  pass: 'text-green-400',
  fail: 'text-red-400',
  error: 'text-amber-400',
}

const VERDICT_ICON: Record<string, string> = {
  pass: '✓',
  fail: '✗',
  error: '!',
}

interface Props {
  result: GraderResult
}

export function GraderResultRow({ result }: Props) {
  const [showActual, setShowActual] = useState(false)
  const hasActual = result.actual_value !== undefined && result.actual_value !== null
  const hasChecklist = result.grader_type === 'checklist' && result.criteria_results && result.criteria_results.length > 0

  return (
    <div className="space-y-1">
      <div className="flex items-start gap-2 text-xs">
        <span className={`font-semibold flex-shrink-0 ${VERDICT_STYLE[result.verdict] ?? 'text-gray-400'}`}>
          {VERDICT_ICON[result.verdict] ?? '?'}
        </span>
        <span className="text-gray-300 font-medium">{result.grader_name}</span>
        <span className="px-1 py-0.5 rounded bg-gray-800 text-gray-500 font-mono text-[10px] flex-shrink-0">
          {GRADER_LABEL[result.grader_type] ?? result.grader_type}
        </span>
        {result.score !== undefined && (
          <span className="text-gray-400 ml-1 flex-shrink-0">
            {Math.round(result.score * 100)}%
          </span>
        )}
        {hasActual && (
          <button
            type="button"
            onClick={() => setShowActual(v => !v)}
            className="text-gray-600 hover:text-gray-400 transition-colors ml-auto flex-shrink-0"
          >
            {showActual ? 'hide value' : 'show value'}
          </button>
        )}
      </div>

      {result.explanation && (
        <p className="text-xs text-gray-500 pl-4">{result.explanation}</p>
      )}

      {showActual && hasActual && (
        <pre className="ml-4 mt-1 rounded bg-gray-800 border border-gray-700 px-3 py-2 text-xs text-gray-300 font-mono overflow-x-auto whitespace-pre-wrap">
          {typeof result.actual_value === 'string'
            ? result.actual_value
            : JSON.stringify(result.actual_value, null, 2)}
        </pre>
      )}

      {hasChecklist && (
        <ChecklistResultDetail
          criteriaResults={result.criteria_results!}
          score={result.score}
        />
      )}
    </div>
  )
}
