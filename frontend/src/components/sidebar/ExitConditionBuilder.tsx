import { useMemo, useCallback } from 'react'
import { useWorkflowStore, getAncestors } from '../../stores/useWorkflowStore'
import { useNodeTypeStore } from '../../stores/useNodeTypeStore'
import type { ExitCondition, ConditionalOperator } from '../../api/types'

interface Props {
  nodeId: string
  config: Record<string, unknown>
  onChange: (config: Record<string, unknown>) => void
  fieldErrors?: Record<string, string>
}

const OPERATORS: { value: ConditionalOperator; label: string }[] = [
  { value: '==',       label: '==' },
  { value: '!=',       label: '!=' },
  { value: '>',        label: '>' },
  { value: '>=',       label: '>=' },
  { value: '<',        label: '<' },
  { value: '<=',       label: '<=' },
  { value: 'contains', label: 'contains' },
]

function detectValueType(raw: string): ExitCondition['value_type'] {
  if (raw === 'true' || raw === 'false') return 'boolean'
  if (raw !== '' && !Number.isNaN(Number(raw))) return 'number'
  return 'string'
}

function emptyCondition(): ExitCondition {
  return { node_id: '', field: '', operator: '==', value: '', value_type: 'string' }
}

// escapeCEL escapes backslashes and double-quotes so the value is safe to
// embed inside a CEL double-quoted string literal.
function escapeCEL(s: string): string {
  return s.replace(/\\/g, '\\\\').replace(/"/g, '\\"')
}

// Generates a CEL string from structured conditions. Only includes conditions
// where both node_id and field are set. Returns '' when nothing is configured.
function buildCEL(conditions: ExitCondition[], logic: 'AND' | 'OR'): string {
  const valid = conditions.filter(c => c.node_id && c.field)
  if (valid.length === 0) return ''
  const clauses = valid.map(c => {
    const lhs = `ctx["${escapeCEL(c.node_id)}"]["${escapeCEL(c.field)}"]`
    if (c.operator === 'contains') {
      return `${lhs}.contains("${escapeCEL(c.value)}")`
    }
    const rhs = c.value_type === 'string' ? `"${escapeCEL(c.value)}"` : c.value
    return `${lhs} ${c.operator} ${rhs}`
  })
  return clauses.join(logic === 'AND' ? ' && ' : ' || ')
}

// ---------------------------------------------------------------------------
// Condition row
// ---------------------------------------------------------------------------

interface ConditionRowProps {
  condition: ExitCondition
  ancestors: { id: string; label: string; fields: string[] }[]
  onChange: (c: ExitCondition) => void
  onRemove: () => void
  removable: boolean
}

function ConditionRow({ condition, ancestors, onChange, onRemove, removable }: ConditionRowProps) {
  const fieldsForNode = useMemo(
    () => ancestors.find(a => a.id === condition.node_id)?.fields ?? [],
    [ancestors, condition.node_id],
  )

  const inputCls =
    'w-full bg-gray-700 border border-gray-600 text-gray-100 text-xs rounded px-2 py-1.5 focus:outline-none focus:border-indigo-500'
  const selectCls =
    'w-full appearance-none bg-gray-700 border border-gray-600 text-gray-100 text-xs rounded px-2 py-1.5 focus:outline-none focus:border-indigo-500 cursor-pointer'
  const labelCls = 'block text-[10px] text-gray-500 mb-0.5'

  return (
    <div className="space-y-1.5 rounded bg-gray-800 border border-gray-700 p-2">
      {/* Node + remove */}
      <div className="flex items-end gap-1.5">
        <div className="flex-1 min-w-0">
          <label className={labelCls}>Node ▾</label>
          <select
            value={condition.node_id}
            onChange={e => onChange({ ...condition, node_id: e.target.value, field: '' })}
            className={selectCls}
          >
            <option value="">— select node —</option>
            {ancestors.map(a => (
              <option key={a.id} value={a.id}>{a.label}</option>
            ))}
          </select>
        </div>
        <button
          onClick={onRemove}
          disabled={!removable}
          className="pb-1.5 text-gray-600 hover:text-red-400 transition-colors disabled:opacity-25 disabled:cursor-not-allowed text-sm leading-none"
          title="Remove condition"
        >
          ✕
        </button>
      </div>

      {/* Field */}
      <div>
        <label className={labelCls}>Field ▾</label>
        {condition.node_id && fieldsForNode.length > 0 ? (
          <select
            value={condition.field}
            onChange={e => onChange({ ...condition, field: e.target.value })}
            className={selectCls}
          >
            <option value="">— select field —</option>
            {fieldsForNode.map(f => (
              <option key={f} value={f}>{f}</option>
            ))}
          </select>
        ) : (
          <input
            type="text"
            value={condition.field}
            onChange={e => onChange({ ...condition, field: e.target.value })}
            className={inputCls}
            placeholder="field name"
          />
        )}
      </div>

      {/* Operator + Value */}
      <div className="flex gap-1.5">
        <div className="w-28 flex-shrink-0">
          <label className={labelCls}>Operator ▾</label>
          <select
            value={condition.operator}
            onChange={e => onChange({ ...condition, operator: e.target.value as ConditionalOperator })}
            className={selectCls}
          >
            {OPERATORS.map(op => (
              <option key={op.value} value={op.value}>{op.label}</option>
            ))}
          </select>
        </div>
        <div className="flex-1 min-w-0">
          <label className={labelCls}>
            Value
            {condition.value !== '' && (
              <span className="ml-1 text-gray-600">({condition.value_type})</span>
            )}
          </label>
          <input
            type="text"
            value={condition.value}
            onChange={e => {
              const value = e.target.value
              onChange({ ...condition, value, value_type: detectValueType(value) })
            }}
            className={inputCls}
            placeholder="value"
          />
        </div>
      </div>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------

export function ExitConditionBuilder({ nodeId, config, onChange, fieldErrors }: Props) {
  const edges        = useWorkflowStore(s => s.edges)
  const nodes        = useWorkflowStore(s => s.nodes)
  const outputParsers = useWorkflowStore(s => s.outputParsers)
  const byTypeId     = useNodeTypeStore(s => s.byTypeId)

  // Ancestor nodes with their output fields, plus synthetic _loop_state entry.
  const ancestors = useMemo(() => {
    const upstream = getAncestors(nodeId, edges)
      .map(id => {
        const rfNode = nodes.find(n => n.id === id)
        if (!rfNode) return null
        const meta = byTypeId(rfNode.data.type_id)
        const props =
          ((meta?.output_schema as Record<string, unknown> | undefined)?.properties as
            Record<string, unknown> | undefined) ?? {}
        const schemaFields = Object.keys(props)
        const parserFields = Object.keys(outputParsers[id] ?? {})
        return {
          id,
          label: (rfNode.data.label as string) || id,
          fields: [...new Set([...schemaFields, ...parserFields])],
        }
      })
      .filter(Boolean) as { id: string; label: string; fields: string[] }[]

    // _loop_state is injected by the engine before each controller invocation.
    return [
      { id: '_loop_state', label: '↩ Loop State', fields: ['iteration'] },
      ...upstream,
    ]
  }, [nodeId, edges, nodes, outputParsers, byTypeId])

  const rawCEL      = config['exit_condition'] as string | undefined
  const hasStructured = 'exit_conditions' in config

  // --- Raw / legacy CEL mode ---
  // Present when exit_condition is a non-empty string but no structured data exists.
  const handleSwitchToBuilder = useCallback(() => {
    if (!window.confirm('Switch to the visual builder? The existing CEL expression will be cleared and cannot be recovered.')) {
      return
    }
    onChange({
      ...config,
      exit_conditions: [emptyCondition()],
      exit_logic: 'AND',
      exit_condition: '',
    })
  }, [config, onChange])

  const handleRawChange = useCallback((expr: string) => {
    onChange({ ...config, exit_condition: expr })
  }, [config, onChange])

  if (rawCEL !== undefined && rawCEL !== '' && !hasStructured) {
    return (
      <div className="space-y-2">
        <div className="rounded-md border border-amber-700 bg-amber-900/30 px-3 py-2">
          <p className="text-xs font-semibold text-amber-400 mb-1">Raw CEL expression</p>
          <p className="text-[10px] text-amber-300 mb-2">
            This exit condition uses a raw CEL expression. Switch to the visual builder to configure it without writing CEL.
          </p>
          <textarea
            value={rawCEL}
            onChange={e => handleRawChange(e.target.value)}
            rows={3}
            className="w-full bg-gray-800 border border-gray-600 text-gray-100 text-xs font-mono rounded px-2 py-1.5 focus:outline-none focus:border-indigo-500 resize-none"
            placeholder='ctx["_loop_state"]["iteration"] >= 3'
          />
          {fieldErrors?.exit_condition && (
            <p className="text-xs text-red-400 mt-1">{fieldErrors.exit_condition}</p>
          )}
          <button
            onClick={handleSwitchToBuilder}
            className="mt-1.5 text-[10px] text-indigo-400 hover:text-indigo-300 transition-colors underline underline-offset-2"
          >
            Switch to visual builder
          </button>
        </div>
      </div>
    )
  }

  // --- Visual builder mode ---
  const conditions = (config['exit_conditions'] as ExitCondition[] | undefined) ?? []
  const logic      = (config['exit_logic'] as 'AND' | 'OR' | undefined) ?? 'AND'

  const commit = (nextConditions: ExitCondition[], nextLogic: 'AND' | 'OR') => {
    onChange({
      ...config,
      exit_conditions: nextConditions,
      exit_logic: nextLogic,
      exit_condition: buildCEL(nextConditions, nextLogic),
    })
  }

  const updateCondition = (i: number, c: ExitCondition) => {
    commit(conditions.map((old, idx) => (idx === i ? c : old)), logic)
  }

  const addCondition = () => {
    commit([...conditions, emptyCondition()], logic)
  }

  const removeCondition = (i: number) => {
    commit(conditions.filter((_, idx) => idx !== i), logic)
  }

  const updateLogic = (next: 'AND' | 'OR') => {
    commit(conditions, next)
  }

  // Derived CEL preview (shown so the user can see what will be sent to the backend).
  const celPreview = buildCEL(conditions, logic)

  return (
    <div className="space-y-2">
      {fieldErrors?.exit_condition && (
        <div className="rounded-md bg-red-900/40 border border-red-700 px-2 py-1.5">
          <p className="text-xs text-red-300">{fieldErrors.exit_condition}</p>
        </div>
      )}

      {/* Logic toggle (only visible when ≥2 conditions) */}
      {conditions.length >= 2 && (
        <div className="flex items-center gap-2">
          <span className="text-[10px] text-gray-500">Combine with</span>
          <div className="flex rounded border border-gray-600 overflow-hidden">
            {(['AND', 'OR'] as const).map(l => (
              <button
                key={l}
                onClick={() => updateLogic(l)}
                className={[
                  'text-[10px] px-2.5 py-0.5 transition-colors',
                  logic === l
                    ? 'bg-indigo-700 text-white'
                    : 'bg-gray-700 text-gray-400 hover:bg-gray-600',
                ].join(' ')}
              >
                {l}
              </button>
            ))}
          </div>
        </div>
      )}

      {/* Condition rows */}
      <div className="space-y-1.5">
        {conditions.map((c, i) => (
          <div key={i}>
            {i > 0 && (
              <div className="text-[10px] text-gray-500 font-semibold px-1 py-0.5">{logic}</div>
            )}
            <ConditionRow
              condition={c}
              ancestors={ancestors}
              onChange={updated => updateCondition(i, updated)}
              onRemove={() => removeCondition(i)}
              removable={conditions.length > 1}
            />
          </div>
        ))}
      </div>

      <button
        onClick={addCondition}
        className="text-[10px] text-indigo-400 hover:text-indigo-300 transition-colors"
      >
        + Add condition
      </button>

      {/* CEL preview — read-only, helps advanced users verify the expression */}
      {celPreview && (
        <div className="rounded border border-gray-700 bg-gray-900/60 px-2 py-1.5">
          <p className="text-[10px] text-gray-500 mb-0.5">Generated CEL</p>
          <code className="text-[10px] text-amber-300 font-mono break-all">{celPreview}</code>
        </div>
      )}

      {/* Empty state hint */}
      {conditions.length === 0 && (
        <p className="text-[10px] text-gray-600 italic">
          No exit conditions — loop runs exactly max_iterations times.
        </p>
      )}
    </div>
  )
}
