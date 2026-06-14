import { useState } from 'react'
import type { GraderDef, GraderScope, GraderType } from '../../api/types'
import type { NodeOption } from './MockEditor'
import { GraderTypeFields } from './GraderTypeFields'

const GRADER_TYPES: { value: GraderType; label: string }[] = [
  { value: 'string_match', label: 'String Match' },
  { value: 'numeric_threshold', label: 'Numeric Threshold' },
  { value: 'llm_judge', label: 'LLM Judge' },
  { value: 'json_schema', label: 'JSON Schema' },
  { value: 'checklist', label: 'Checklist' },
]

const selectCls =
  'w-full rounded-md bg-gray-900 border border-gray-600 px-3 py-1.5 text-sm text-gray-100 focus:outline-none focus:border-indigo-500'

const inputCls =
  'w-full rounded-md bg-gray-900 border border-gray-600 px-3 py-1.5 text-sm text-gray-100 placeholder-gray-500 focus:outline-none focus:border-indigo-500'

interface Props {
  grader: GraderDef
  nodes: NodeOption[]
  onChange: (grader: GraderDef) => void
  onRemove: () => void
  fieldErrors?: Record<string, string>
}

export function GraderEditor({ grader, nodes, onChange, onRemove, fieldErrors }: Props) {
  const [expanded, setExpanded] = useState(true)

  const set = <K extends keyof GraderDef>(k: K, v: GraderDef[K]) =>
    onChange({ ...grader, [k]: v })

  const scopeLabel = grader.scope === 'node' ? 'Node' : 'Workflow'

  const safeNodes = (nodes ?? []).filter(
    (n): n is NodeOption => n != null && typeof n === 'object' && typeof n.id === 'string',
  )

  return (
    <div className="border border-gray-700 rounded-lg overflow-hidden">
      {/* Header row */}
      <div className="flex items-center gap-2 px-3 py-2 bg-gray-800 cursor-pointer" onClick={() => setExpanded(e => !e)}>
        <span className="text-gray-400 text-xs w-3">{expanded ? '▾' : '▸'}</span>
        <span className="flex-1 text-sm font-medium text-gray-200 truncate">
          {grader.name || <span className="text-gray-500 italic">Unnamed grader</span>}
        </span>
        <span className="text-xs px-1.5 py-0.5 rounded bg-gray-700 text-gray-400">
          {GRADER_TYPES.find(t => t.value === grader.type)?.label ?? grader.type}
        </span>
        <span className="text-xs px-1.5 py-0.5 rounded bg-gray-700 text-gray-400">
          {scopeLabel}
        </span>
        <button
          type="button"
          onClick={e => { e.stopPropagation(); onRemove() }}
          className="text-gray-500 hover:text-red-400 transition-colors text-xs px-1"
          title="Remove grader"
        >
          ✕
        </button>
      </div>

      {/* Body */}
      {expanded && (
        <div className="p-3 space-y-3 bg-gray-850 border-t border-gray-700" style={{ background: '#1a2236' }}>
          {/* Name */}
          <div className="space-y-1">
            <label className="block text-xs font-semibold text-gray-300">Grader name</label>
            <input
              className={inputCls}
              placeholder="e.g. Response is helpful"
              value={grader.name}
              onChange={e => set('name', e.target.value)}
            />
          </div>

          {/* Type */}
          <div className="space-y-1">
            <label className="block text-xs font-semibold text-gray-300">Type</label>
            <select
              className={selectCls}
              value={grader.type}
              onChange={e => {
                const newType = e.target.value as GraderType
                const isLLM = newType === 'llm_judge' || newType === 'checklist'
                // Seed LLM defaults so config.provider is always set before save.
                const config = isLLM && !grader.config.provider
                  ? { ...grader.config, provider: 'anthropic', model: (grader.config.model as string) ?? '' }
                  : grader.config
                onChange({ ...grader, type: newType, config })
              }}
            >
              {GRADER_TYPES.map(t => (
                <option key={t.value} value={t.value}>{t.label}</option>
              ))}
            </select>
          </div>

          {/* Scope */}
          <div className="space-y-1">
            <label className="block text-xs font-semibold text-gray-300">Scope</label>
            <div className="flex gap-2">
              {(['workflow', 'node'] as GraderScope[]).map(s => (
                <button
                  key={s}
                  type="button"
                  onClick={() => set('scope', s)}
                  className={`px-3 py-1 rounded-md text-xs font-medium transition-colors capitalize ${
                    grader.scope === s
                      ? 'bg-indigo-600 text-white'
                      : 'bg-gray-700 text-gray-300 hover:bg-gray-600'
                  }`}
                >
                  {s}
                </button>
              ))}
            </div>
          </div>

          {/* Node selector (only when scope = node) */}
          {grader.scope === 'node' && (
            <div className="space-y-1">
              <label className="block text-xs font-semibold text-gray-300">Target node</label>
              {fieldErrors?.node_id && (
                <p className="text-xs text-red-400">{fieldErrors.node_id}</p>
              )}
              <select
                className={selectCls}
                value={grader.node_id ?? ''}
                onChange={e => set('node_id', e.target.value || undefined)}
              >
                <option value="">— select a node —</option>
                {safeNodes.map(n => (
                  <option key={n.id} value={n.id}>{n.label || n.id} ({n.id})</option>
                ))}
              </select>
            </div>
          )}

          {/* Type-specific fields */}
          <GraderTypeFields
            type={grader.type}
            config={grader.config}
            onChange={config => set('config', config)}
            errors={fieldErrors}
          />
        </div>
      )}
    </div>
  )
}
