import type { CriterionResult } from '../../api/types'

interface Props {
  criteriaResults: CriterionResult[]
  score?: number
}

export function ChecklistResultDetail({ criteriaResults, score }: Props) {
  const met = criteriaResults.filter(c => c.met).length
  const total = criteriaResults.length

  return (
    <div className="ml-4 mt-1 rounded-md border border-gray-700 bg-gray-800/50 overflow-hidden">
      <div className="divide-y divide-gray-700">
        {criteriaResults.map((c, i) => (
          <div key={i} className="flex items-start gap-3 px-3 py-2">
            <span
              className={`text-xs font-semibold flex-shrink-0 mt-0.5 ${c.met ? 'text-green-400' : 'text-red-400'}`}
            >
              {c.met ? '✓' : '✗'}
            </span>
            <div className="min-w-0 flex-1 space-y-0.5">
              <p className="text-xs text-gray-300">{c.criterion}</p>
              {c.explanation && (
                <p className="text-xs text-gray-500">{c.explanation}</p>
              )}
            </div>
          </div>
        ))}
      </div>
      <div className="px-3 py-2 bg-gray-800 border-t border-gray-700">
        <span className="text-xs text-gray-400">
          {met} of {total} {total === 1 ? 'criterion' : 'criteria'} met
          {score !== undefined && ` — ${Math.round(score * 100)}%`}
        </span>
      </div>
    </div>
  )
}
