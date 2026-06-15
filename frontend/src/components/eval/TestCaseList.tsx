import type { TestCase } from '../../api/types'

interface Props {
  testCases: TestCase[]
  onEdit: (tc: TestCase) => void
  onDelete: (tc: TestCase) => void
  onMoveUp: (tc: TestCase) => void
  onMoveDown: (tc: TestCase) => void
  deleting?: string | null
}

export function TestCaseList({ testCases, onEdit, onDelete, onMoveUp, onMoveDown, deleting }: Props) {
  if (testCases.length === 0) {
    return (
      <p className="text-sm text-gray-500 italic py-4 text-center">
        No test cases yet. Click "Add Test Case" to create one.
      </p>
    )
  }

  return (
    <div className="space-y-2">
      {testCases.map((tc, i) => (
        <div
          key={tc.id}
          className="flex items-center gap-3 rounded-lg bg-gray-800 border border-gray-700 px-4 py-3 hover:bg-gray-700 transition-colors group"
          style={{ background: deleting === tc.id ? '#1f1f1f' : undefined }}
        >
          {/* Reorder buttons */}
          <div className="flex flex-col gap-0.5 flex-shrink-0">
            <button
              type="button"
              onClick={() => onMoveUp(tc)}
              disabled={i === 0}
              className="text-gray-600 hover:text-gray-300 disabled:opacity-20 disabled:cursor-not-allowed transition-colors text-xs leading-none"
              title="Move up"
            >
              ▲
            </button>
            <button
              type="button"
              onClick={() => onMoveDown(tc)}
              disabled={i === testCases.length - 1}
              className="text-gray-600 hover:text-gray-300 disabled:opacity-20 disabled:cursor-not-allowed transition-colors text-xs leading-none"
              title="Move down"
            >
              ▼
            </button>
          </div>

          {/* Test case info */}
          <div className="flex-1 min-w-0">
            <div className="text-sm font-medium text-gray-100 truncate">{tc.name}</div>
            <div className="flex items-center gap-2 mt-0.5">
              {tc.description && (
                <span className="text-xs text-gray-500 truncate max-w-xs">{tc.description}</span>
              )}
              {tc.graders.length > 0 && (
                <span className="text-xs px-1.5 py-0.5 rounded-full bg-indigo-900/60 text-indigo-300 flex-shrink-0">
                  {tc.graders.length} grader{tc.graders.length !== 1 ? 's' : ''}
                </span>
              )}
              {tc.mocks.length > 0 && (
                <span className="text-xs px-1.5 py-0.5 rounded-full bg-gray-700 text-gray-400 flex-shrink-0">
                  {tc.mocks.length} mock{tc.mocks.length !== 1 ? 's' : ''}
                </span>
              )}
            </div>
          </div>

          {/* Actions */}
          <div className="flex gap-2 opacity-0 group-hover:opacity-100 transition-opacity">
            <button
              type="button"
              onClick={() => onEdit(tc)}
              className="text-xs text-indigo-400 hover:text-indigo-300 transition-colors px-2 py-1 rounded hover:bg-gray-700"
            >
              Edit
            </button>
            <button
              type="button"
              onClick={() => onDelete(tc)}
              disabled={deleting === tc.id}
              className="text-xs text-red-500 hover:text-red-400 transition-colors px-2 py-1 rounded hover:bg-gray-700 disabled:opacity-50"
            >
              {deleting === tc.id ? '…' : 'Delete'}
            </button>
          </div>
        </div>
      ))}
    </div>
  )
}
