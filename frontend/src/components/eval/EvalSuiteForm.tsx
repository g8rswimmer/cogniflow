import { useState } from 'react'
import type { EvalSuite } from '../../api/types'

const inputCls =
  'w-full rounded-md bg-gray-900 border border-gray-600 px-3 py-1.5 text-sm text-gray-100 placeholder-gray-500 focus:outline-none focus:border-indigo-500'

interface Props {
  suite?: EvalSuite
  onSave: (data: { name: string; description?: string; pass_threshold: number; max_concurrency: number }) => Promise<void>
  onClose: () => void
  saving?: boolean
  error?: string
}

export function EvalSuiteForm({ suite, onSave, onClose, saving, error }: Props) {
  const [name, setName] = useState(suite?.name ?? '')
  const [description, setDescription] = useState(suite?.description ?? '')
  const [passThreshold, setPassThreshold] = useState(suite?.pass_threshold ?? 1.0)
  const [maxConcurrency, setMaxConcurrency] = useState(suite?.max_concurrency ?? 1)

  const handleSave = async () => {
    await onSave({
      name,
      description: description.trim() || undefined,
      pass_threshold: passThreshold,
      max_concurrency: maxConcurrency,
    })
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60"
      onMouseDown={e => { if (e.target === e.currentTarget) onClose() }}
    >
      <div className="w-full max-w-md bg-gray-800 rounded-xl shadow-2xl border border-gray-700 p-5">
        <div className="flex items-start justify-between mb-4">
          <h2 className="text-sm font-semibold text-gray-100">
            {suite ? 'Edit Suite' : 'New Eval Suite'}
          </h2>
          <button
            onClick={onClose}
            className="text-gray-500 hover:text-gray-300 transition-colors text-sm leading-none"
          >
            ✕
          </button>
        </div>

        {error && (
          <div className="mb-3 rounded-md bg-red-900/40 border border-red-700 px-3 py-2">
            <p className="text-xs text-red-300">{error}</p>
          </div>
        )}

        <div className="space-y-4">
          <div className="space-y-1">
            <label className="block text-xs font-semibold text-gray-300">Name <span className="text-red-400">*</span></label>
            <input
              className={inputCls}
              placeholder="Regression Suite"
              value={name}
              onChange={e => setName(e.target.value)}
              autoFocus
            />
          </div>

          <div className="space-y-1">
            <label className="block text-xs font-semibold text-gray-300">Description (optional)</label>
            <input
              className={inputCls}
              placeholder="What this suite validates…"
              value={description}
              onChange={e => setDescription(e.target.value)}
            />
          </div>

          <div className="space-y-1">
            <label className="block text-xs font-semibold text-gray-300">
              Pass threshold
              <span className="ml-1.5 font-normal text-gray-500">
                — fraction of graders that must pass (0.0–1.0)
              </span>
            </label>
            <input
              type="range"
              min="0"
              max="1"
              step="0.05"
              value={passThreshold}
              onChange={e => setPassThreshold(Number(e.target.value))}
              className="w-full accent-indigo-500"
            />
            <div className="flex justify-between text-xs text-gray-500">
              <span>0%</span>
              <span className="text-gray-200 font-semibold">{Math.round(passThreshold * 100)}%</span>
              <span>100%</span>
            </div>
          </div>

          <div className="space-y-1">
            <label className="block text-xs font-semibold text-gray-300">
              Max concurrency
              <span className="ml-1.5 font-normal text-gray-500">— parallel test cases</span>
            </label>
            <input
              type="number"
              min="1"
              className={inputCls}
              value={maxConcurrency}
              onChange={e => setMaxConcurrency(Math.max(1, Number(e.target.value)))}
            />
          </div>
        </div>

        <div className="flex items-center justify-end gap-2 mt-5">
          <button
            type="button"
            onClick={onClose}
            className="px-3 py-1.5 rounded-md text-xs text-gray-400 hover:text-gray-200 transition-colors"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={handleSave}
            disabled={saving || !name.trim()}
            className="px-4 py-1.5 rounded-md bg-indigo-600 hover:bg-indigo-500 disabled:opacity-50 text-white text-xs font-semibold transition-colors"
          >
            {saving ? 'Saving…' : suite ? 'Update' : 'Create Suite'}
          </button>
        </div>
      </div>
    </div>
  )
}
