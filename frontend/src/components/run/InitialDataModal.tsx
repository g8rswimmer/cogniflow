import { useState, useRef, useEffect } from 'react'

interface SchemaProperty {
  type?: string
  description?: string
  title?: string
}

interface Props {
  schema: Record<string, unknown> | null
  onRun: (initialData: Record<string, unknown>) => void
  onCancel: () => void
}

const inputCls = `
  w-full rounded-md bg-gray-900 border border-gray-600
  px-3 py-1.5 text-sm text-gray-100 placeholder-gray-500
  focus:outline-none focus:border-indigo-500
`.trim()

/** Render a single field input appropriate for its declared type. */
function FieldInput({
  name,
  prop,
  value,
  onChange,
}: {
  name: string
  prop: SchemaProperty
  value: unknown
  onChange: (v: unknown) => void
}) {
  const type = prop.type ?? 'string'
  const label = prop.title ?? name
  const hint = prop.description

  return (
    <div className="space-y-1">
      <label className="block text-xs font-semibold text-gray-300">
        {label}
        <span className="ml-1.5 font-normal text-gray-500 font-mono text-[10px]">{type}</span>
      </label>
      {hint && <p className="text-xs text-gray-500">{hint}</p>}

      {type === 'boolean' ? (
        <label className="flex items-center gap-2 cursor-pointer">
          <input
            type="checkbox"
            checked={!!value}
            onChange={e => onChange(e.target.checked)}
            className="rounded border-gray-600 bg-gray-900 text-indigo-500 focus:ring-indigo-500"
          />
          <span className="text-sm text-gray-300">{value ? 'true' : 'false'}</span>
        </label>
      ) : type === 'number' || type === 'integer' ? (
        <input
          type="number"
          value={value === undefined || value === null ? '' : String(value)}
          onChange={e => {
            const n = e.target.value === '' ? undefined : Number(e.target.value)
            onChange(n)
          }}
          placeholder="0"
          className={inputCls}
        />
      ) : (
        <input
          type="text"
          value={value === undefined || value === null ? '' : String(value)}
          onChange={e => onChange(e.target.value)}
          placeholder={`Enter ${label.toLowerCase()}…`}
          className={inputCls}
        />
      )}
    </div>
  )
}

export function InitialDataModal({ schema, onRun, onCancel }: Props) {
  const [text, setText] = useState('{\n\n}')
  const [error, setError] = useState<string | null>(null)
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  // Parse schema properties for the guided form
  const properties = schema
    ? ((schema.properties as Record<string, SchemaProperty> | undefined) ?? {})
    : {}
  const fieldNames = Object.keys(properties)
  const hasSchema = fieldNames.length > 0

  // Form values for the guided path
  const [formValues, setFormValues] = useState<Record<string, unknown>>(() => {
    const init: Record<string, unknown> = {}
    for (const [name, prop] of Object.entries(properties)) {
      init[name] = (prop.type === 'boolean') ? false
        : (prop.type === 'number' || prop.type === 'integer') ? undefined
        : ''
    }
    return init
  })

  useEffect(() => {
    if (hasSchema) return
    const el = textareaRef.current
    if (!el) return
    el.focus()
    el.setSelectionRange(2, 2)
  }, [hasSchema])

  const handleRunGuided = () => {
    // Strip empty strings/undefined so the payload is clean
    const data: Record<string, unknown> = {}
    for (const [k, v] of Object.entries(formValues)) {
      if (v !== undefined && v !== '') data[k] = v
    }
    onRun(data)
  }

  const handleRunRaw = () => {
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
    if (!hasSchema && e.key === 'Enter' && (e.metaKey || e.ctrlKey)) handleRunRaw()
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60"
      onMouseDown={e => { if (e.target === e.currentTarget) onCancel() }}
      onKeyDown={handleKeyDown}
    >
      <div className="w-full max-w-lg bg-gray-800 rounded-xl shadow-2xl border border-gray-700 p-5">
        <div className="flex items-start justify-between mb-1">
          <h2 className="text-sm font-semibold text-gray-100">Initial Run Data</h2>
          <button
            onClick={onCancel}
            className="text-gray-500 hover:text-gray-300 transition-colors text-sm leading-none"
          >
            ✕
          </button>
        </div>

        <p className="text-xs text-gray-400 mb-4">
          {hasSchema
            ? <>Values are passed to the workflow as <code className="text-indigo-300 bg-gray-900 px-1 rounded">_initial</code>. Nodes reference them with <code className="text-indigo-300 bg-gray-900 px-1 rounded">{'{{._initial.field}}'}</code>.</>
            : <>JSON object passed as <code className="text-indigo-300 bg-gray-900 px-1 rounded">_initial</code>. Reference fields with <code className="text-indigo-300 bg-gray-900 px-1 rounded">{'{{._initial.key}}'}</code>. Leave <code className="text-indigo-300 bg-gray-900 px-1 rounded">{'{}'}</code> to run with no initial data.</>
          }
        </p>

        {hasSchema ? (
          /* Guided form: one typed input per declared field */
          <div className="space-y-3 max-h-80 overflow-y-auto pr-1">
            {fieldNames.map(name => (
              <FieldInput
                key={name}
                name={name}
                prop={properties[name]}
                value={formValues[name]}
                onChange={v => setFormValues(prev => ({ ...prev, [name]: v }))}
              />
            ))}
          </div>
        ) : (
          /* Free-form JSON textarea */
          <textarea
            ref={textareaRef}
            value={text}
            onChange={e => { setText(e.target.value); setError(null) }}
            rows={8}
            spellCheck={false}
            className="
              w-full rounded-md bg-gray-900 border border-gray-600
              px-3 py-2 font-mono text-sm text-gray-100 placeholder-gray-600
              focus:outline-none focus:border-indigo-500 resize-y
            "
            placeholder='{}'
          />
        )}

        {error && (
          <p className="text-xs text-red-400 mt-1.5">{error}</p>
        )}

        <div className="flex items-center justify-between mt-4">
          <span className="text-xs text-gray-600">
            {!hasSchema && 'Cmd/Ctrl + Enter to run'}
          </span>
          <div className="flex gap-2">
            <button
              type="button"
              onClick={onCancel}
              className="px-3 py-1.5 rounded-md text-xs text-gray-400 hover:text-gray-200 transition-colors"
            >
              Cancel
            </button>
            <button
              type="button"
              onClick={hasSchema ? handleRunGuided : handleRunRaw}
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
