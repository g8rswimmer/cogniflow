import { useState, useRef, useEffect } from 'react'

interface Props {
  onRun: (initialData: Record<string, unknown>) => void
  onCancel: () => void
}

export function InitialDataModal({ onRun, onCancel }: Props) {
  const [text, setText] = useState('{\n\n}')
  const [error, setError] = useState<string | null>(null)
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    // Focus and place cursor on the blank line between the braces
    const el = textareaRef.current
    if (!el) return
    el.focus()
    el.setSelectionRange(2, 2)
  }, [])

  const handleRun = () => {
    const trimmed = text.trim() === '' ? '{}' : text
    try {
      const parsed = JSON.parse(trimmed)
      if (typeof parsed !== 'object' || Array.isArray(parsed) || parsed === null) {
        setError('Initial data must be a JSON object { ... }')
        return
      }
      onRun(parsed as Record<string, unknown>)
    } catch {
      setError('Invalid JSON — check your syntax and try again')
    }
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Escape') onCancel()
    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) handleRun()
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60"
      onMouseDown={e => { if (e.target === e.currentTarget) onCancel() }}
    >
      <div className="w-full max-w-lg bg-gray-800 rounded-xl shadow-2xl border border-gray-700 p-5">
        <h2 className="text-sm font-semibold text-gray-100 mb-1">Initial Run Data</h2>
        <p className="text-xs text-gray-400 mb-3">
          JSON object passed as{' '}
          <code className="text-indigo-300 bg-gray-900 px-1 rounded">_initial</code> — reference
          fields in templates with{' '}
          <code className="text-indigo-300 bg-gray-900 px-1 rounded">{'{{._initial.key}}'}</code>.
          Leave <code className="text-indigo-300 bg-gray-900 px-1 rounded">{'{}'}</code> to run
          with no initial data.
        </p>

        <textarea
          ref={textareaRef}
          value={text}
          onChange={e => { setText(e.target.value); setError(null) }}
          onKeyDown={handleKeyDown}
          rows={8}
          spellCheck={false}
          className="
            w-full rounded-md bg-gray-900 border border-gray-600
            px-3 py-2 font-mono text-sm text-gray-100 placeholder-gray-600
            focus:outline-none focus:border-indigo-500 resize-y
          "
          placeholder='{}'
        />

        {error && (
          <p className="text-xs text-red-400 mt-1.5">{error}</p>
        )}

        <div className="flex items-center justify-between mt-3">
          <span className="text-xs text-gray-600">Cmd/Ctrl + Enter to run</span>
          <div className="flex gap-2">
            <button
              onClick={onCancel}
              className="px-3 py-1.5 rounded-md text-xs text-gray-400 hover:text-gray-200 transition-colors"
            >
              Cancel
            </button>
            <button
              onClick={handleRun}
              className="px-4 py-1.5 rounded-md bg-green-700 hover:bg-green-600 text-white text-xs font-semibold transition-colors"
            >
              Run
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
