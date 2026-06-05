import { useState } from 'react'
import { useWorkflowStore } from '../../stores/useWorkflowStore'
import { useNodeTypeStore } from '../../stores/useNodeTypeStore'
import type { OutputParser, OutputParserKind } from '../../api/types'

interface Props {
  nodeId: string
  typeId: string
}

const emptyParser: Omit<OutputParser, 'kind'> & { kind: OutputParserKind; name: string } = {
  name: '',
  kind: 'json_path',
  source: '',
  pattern: '',
  capture_group: 0,
}

export function OutputParserPanel({ nodeId, typeId }: Props) {
  const parsers = useWorkflowStore(s => s.outputParsers[nodeId]) ?? {}
  const updateOutputParsers = useWorkflowStore(s => s.updateOutputParsers)
  const byTypeId = useNodeTypeStore(s => s.byTypeId)

  const [adding, setAdding] = useState(false)
  const [draft, setDraft] = useState({ ...emptyParser })
  const [error, setError] = useState<string | null>(null)

  const meta = byTypeId(typeId)
  const outputSchema = (meta?.output_schema ?? {}) as Record<string, unknown>
  const outputProperties =
    (outputSchema.properties as Record<string, unknown> | undefined) ?? {}
  const sourceOptions = Object.keys(outputProperties)

  const handleDelete = (name: string) => {
    const next = { ...parsers }
    delete next[name]
    updateOutputParsers(nodeId, next)
  }

  const handleAdd = () => {
    setError(null)
    if (!draft.name.trim()) { setError('Name is required'); return }
    if (!draft.source) { setError('Source field is required'); return }
    if (!draft.pattern.trim()) { setError('Pattern is required'); return }
    if (parsers[draft.name]) { setError(`"${draft.name}" already exists`); return }

    const parser: OutputParser = {
      kind: draft.kind,
      source: draft.source,
      pattern: draft.pattern.trim(),
      ...(draft.kind === 'regex' ? { capture_group: draft.capture_group } : {}),
    }
    updateOutputParsers(nodeId, { ...parsers, [draft.name.trim()]: parser })
    setDraft({ ...emptyParser })
    setAdding(false)
  }

  return (
    <div className="mt-4">
      <div className="flex items-center justify-between mb-2">
        <span className="text-xs font-semibold uppercase tracking-wider text-gray-400">
          Output Parsers
        </span>
        {!adding && (
          <button
            onClick={() => { setAdding(true); setError(null) }}
            className="text-xs text-indigo-400 hover:text-indigo-300 transition-colors"
          >
            + Add Extractor
          </button>
        )}
      </div>

      {/* Existing parsers */}
      {Object.entries(parsers).map(([name, p]) => (
        <div
          key={name}
          className="flex items-start justify-between rounded-md bg-gray-700/60 border border-gray-600 px-2 py-1.5 mb-1"
        >
          <div className="min-w-0">
            <span className="text-sm font-mono text-gray-100">{name}</span>
            <div className="text-xs text-gray-400 mt-0.5">
              {p.kind} · {p.source} · <span className="font-mono">{p.pattern}</span>
              {p.kind === 'regex' && ` · group ${p.capture_group ?? 0}`}
            </div>
          </div>
          <button
            onClick={() => handleDelete(name)}
            className="ml-2 text-gray-500 hover:text-red-400 transition-colors flex-shrink-0"
            title="Remove"
          >
            ✕
          </button>
        </div>
      ))}

      {/* Add form */}
      {adding && (
        <div className="rounded-md border border-gray-600 bg-gray-700/60 p-2 space-y-2 mt-1">
          <Field label="Name">
            <input
              type="text"
              value={draft.name}
              onChange={e => setDraft(d => ({ ...d, name: e.target.value }))}
              placeholder="extracted_field"
              className={inputClass}
            />
          </Field>

          <Field label="Source field">
            <select
              value={draft.source}
              onChange={e => setDraft(d => ({ ...d, source: e.target.value }))}
              className={inputClass}
            >
              <option value="">Select…</option>
              {sourceOptions.map(s => (
                <option key={s} value={s}>{s}</option>
              ))}
            </select>
          </Field>

          <Field label="Type">
            <select
              value={draft.kind}
              onChange={e => setDraft(d => ({ ...d, kind: e.target.value as OutputParserKind }))}
              className={inputClass}
            >
              <option value="json_path">json_path</option>
              <option value="regex">regex</option>
            </select>
          </Field>

          <Field label="Pattern">
            <input
              type="text"
              value={draft.pattern}
              onChange={e => setDraft(d => ({ ...d, pattern: e.target.value }))}
              placeholder={draft.kind === 'json_path' ? 'status' : '(\\d+)'}
              className={inputClass}
            />
          </Field>

          {draft.kind === 'regex' && (
            <Field label="Capture group">
              <input
                type="number"
                min={0}
                value={draft.capture_group ?? 0}
                onChange={e => setDraft(d => ({ ...d, capture_group: Number(e.target.value) }))}
                className={inputClass}
              />
            </Field>
          )}

          {error && <p className="text-xs text-red-400">{error}</p>}

          <div className="flex gap-2 pt-1">
            <button
              onClick={handleAdd}
              className="flex-1 rounded bg-indigo-600 hover:bg-indigo-500 text-white text-xs py-1.5 transition-colors"
            >
              Add
            </button>
            <button
              onClick={() => { setAdding(false); setError(null); setDraft({ ...emptyParser }) }}
              className="flex-1 rounded bg-gray-600 hover:bg-gray-500 text-gray-200 text-xs py-1.5 transition-colors"
            >
              Cancel
            </button>
          </div>
        </div>
      )}

      {Object.keys(parsers).length === 0 && !adding && (
        <p className="text-xs text-gray-500 italic">No extractors defined</p>
      )}
    </div>
  )
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <label className="text-xs text-gray-400 block mb-0.5">{label}</label>
      {children}
    </div>
  )
}

const inputClass = `
  w-full rounded bg-gray-800 border border-gray-600
  px-2 py-1 text-sm text-gray-100
  focus:outline-none focus:border-indigo-500
`
