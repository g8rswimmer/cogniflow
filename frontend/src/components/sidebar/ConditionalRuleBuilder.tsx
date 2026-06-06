import { useMemo, useCallback } from 'react'
import { useWorkflowStore, getAncestors } from '../../stores/useWorkflowStore'
import { useNodeTypeStore } from '../../stores/useNodeTypeStore'
import type {
  ConditionalRule,
  ConditionalCondition,
  ConditionalOperator,
} from '../../api/types'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface Props {
  nodeId: string
  config: Record<string, unknown>
  onChange: (config: Record<string, unknown>) => void
  fieldErrors?: Record<string, string>
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const OPERATORS: { value: ConditionalOperator; label: string }[] = [
  { value: '==',       label: '==' },
  { value: '!=',       label: '!=' },
  { value: '>',        label: '>' },
  { value: '>=',       label: '>=' },
  { value: '<',        label: '<' },
  { value: '<=',       label: '<=' },
  { value: 'contains', label: 'contains' },
]

function detectValueType(raw: string): ConditionalCondition['value_type'] {
  if (raw === 'true' || raw === 'false') return 'boolean'
  if (raw !== '' && !Number.isNaN(Number(raw))) return 'number'
  return 'string'
}

function emptyCondition(): ConditionalCondition {
  return { node_id: '', field: '', operator: '==', value: '', value_type: 'string' }
}

function emptyRule(index: number): ConditionalRule {
  return { label: `rule_${index + 1}`, logic: 'AND', conditions: [emptyCondition()] }
}

// ---------------------------------------------------------------------------
// Condition row
// ---------------------------------------------------------------------------

interface ConditionRowProps {
  condition: ConditionalCondition
  ancestors: { id: string; label: string; fields: string[] }[]
  onChange: (c: ConditionalCondition) => void
  onRemove: () => void
  removable: boolean
}

function ConditionRow({ condition, ancestors, onChange, onRemove, removable }: ConditionRowProps) {
  const fieldsForNode = useMemo(() => {
    return ancestors.find(a => a.id === condition.node_id)?.fields ?? []
  }, [ancestors, condition.node_id])

  const setNode = useCallback((nodeId: string) => {
    onChange({ ...condition, node_id: nodeId, field: '' })
  }, [condition, onChange])

  const setField = useCallback((field: string) => {
    onChange({ ...condition, field })
  }, [condition, onChange])

  const setOperator = useCallback((operator: string) => {
    onChange({ ...condition, operator: operator as ConditionalOperator })
  }, [condition, onChange])

  const setValue = useCallback((value: string) => {
    onChange({ ...condition, value, value_type: detectValueType(value) })
  }, [condition, onChange])

  const inputCls =
    'w-full bg-gray-700 border border-gray-600 text-gray-100 text-xs rounded px-2 py-1.5 focus:outline-none focus:border-indigo-500'
  // appearance-none removes the browser's default chrome so our bg/text colours
  // apply reliably on macOS Safari; the ▾ glyph replaces the native arrow.
  const selectCls =
    'w-full appearance-none bg-gray-700 border border-gray-600 text-gray-100 text-xs rounded px-2 py-1.5 focus:outline-none focus:border-indigo-500 cursor-pointer'
  const labelCls = 'block text-[10px] text-gray-500 mb-0.5'

  return (
    <div className="space-y-1.5 rounded bg-gray-800 border border-gray-700 p-2">
      {/* Node + remove button */}
      <div className="flex items-end gap-1.5">
        <div className="flex-1 min-w-0">
          <label className={labelCls}>Node ▾</label>
          <select value={condition.node_id} onChange={e => setNode(e.target.value)} className={selectCls}>
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
          <select value={condition.field} onChange={e => setField(e.target.value)} className={selectCls}>
            <option value="">— select field —</option>
            {fieldsForNode.map(f => (
              <option key={f} value={f}>{f}</option>
            ))}
          </select>
        ) : (
          <input
            type="text"
            value={condition.field}
            onChange={e => setField(e.target.value)}
            className={inputCls}
            placeholder="field name"
          />
        )}
      </div>

      {/* Operator + Value side by side — both wide enough to read */}
      <div className="flex gap-1.5">
        <div className="w-28 flex-shrink-0">
          <label className={labelCls}>Operator ▾</label>
          <select value={condition.operator} onChange={e => setOperator(e.target.value)} className={selectCls}>
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
            onChange={e => setValue(e.target.value)}
            className={inputCls}
            placeholder="value"
          />
        </div>
      </div>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Rule card
// ---------------------------------------------------------------------------

interface RuleCardProps {
  rule: ConditionalRule
  index: number
  total: number
  labelError?: string
  ancestors: { id: string; label: string; fields: string[] }[]
  onChange: (r: ConditionalRule) => void
  onRemove: () => void
  onMoveUp: () => void
  onMoveDown: () => void
}

function RuleCard({ rule, index, total, labelError, ancestors, onChange, onRemove, onMoveUp, onMoveDown }: RuleCardProps) {
  const updateLabel = (label: string) => onChange({ ...rule, label })
  const updateLogic  = (logic: 'AND' | 'OR') => onChange({ ...rule, logic })

  const updateCondition = (i: number, c: ConditionalCondition) => {
    const conditions = rule.conditions.map((old, idx) => (idx === i ? c : old))
    onChange({ ...rule, conditions })
  }
  const addCondition = () => onChange({ ...rule, conditions: [...rule.conditions, emptyCondition()] })
  const removeCondition = (i: number) => {
    const conditions = rule.conditions.filter((_, idx) => idx !== i)
    onChange({ ...rule, conditions })
  }

  return (
    <div className="rounded-md border border-gray-600 bg-gray-900/50 p-2 space-y-2">
      {/* Rule header */}
      <div className="flex items-center gap-1.5">
        {/* Reorder buttons */}
        <div className="flex flex-col">
          <button
            onClick={onMoveUp}
            disabled={index === 0}
            className="text-gray-600 hover:text-gray-300 disabled:opacity-20 leading-none text-xs"
            title="Move up"
          >▲</button>
          <button
            onClick={onMoveDown}
            disabled={index === total - 1}
            className="text-gray-600 hover:text-gray-300 disabled:opacity-20 leading-none text-xs"
            title="Move down"
          >▼</button>
        </div>

        {/* Label input */}
        <div className="flex-1 min-w-0">
          <input
            type="text"
            value={rule.label}
            onChange={e => updateLabel(e.target.value)}
            className={[
              'w-full bg-gray-700 border text-gray-100 text-xs rounded px-1.5 py-1 focus:outline-none focus:border-indigo-500',
              labelError ? 'border-red-500' : 'border-gray-600',
            ].join(' ')}
            placeholder="rule label"
          />
          {labelError && <p className="text-red-400 text-[10px] mt-0.5">{labelError}</p>}
        </div>

        {/* Logic toggle */}
        <div className="flex rounded border border-gray-600 overflow-hidden flex-shrink-0">
          {(['AND', 'OR'] as const).map(l => (
            <button
              key={l}
              onClick={() => updateLogic(l)}
              className={[
                'text-[10px] px-2 py-0.5 transition-colors',
                rule.logic === l
                  ? 'bg-indigo-700 text-white'
                  : 'bg-gray-700 text-gray-400 hover:bg-gray-600',
              ].join(' ')}
            >
              {l}
            </button>
          ))}
        </div>

        {/* Remove rule */}
        <button
          onClick={onRemove}
          disabled={total <= 1}
          className="text-gray-600 hover:text-red-400 transition-colors disabled:opacity-20 disabled:cursor-not-allowed text-sm flex-shrink-0"
          title="Remove rule"
        >
          ✕
        </button>
      </div>

      {/* Conditions */}
      <div className="space-y-1.5">
        {rule.conditions.map((c, i) => (
          <div key={i}>
            {i > 0 && (
              <div className="text-[10px] text-gray-500 font-semibold px-1">{rule.logic}</div>
            )}
            <ConditionRow
              condition={c}
              ancestors={ancestors}
              onChange={updated => updateCondition(i, updated)}
              onRemove={() => removeCondition(i)}
              removable={rule.conditions.length > 1}
            />
          </div>
        ))}
        <button
          onClick={addCondition}
          className="text-[10px] text-indigo-400 hover:text-indigo-300 transition-colors"
        >
          + Add condition
        </button>
      </div>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------

export function ConditionalRuleBuilder({ nodeId, config, onChange, fieldErrors }: Props) {
  const edges       = useWorkflowStore(s => s.edges)
  const nodes       = useWorkflowStore(s => s.nodes)
  const outputParsers = useWorkflowStore(s => s.outputParsers)
  const byTypeId    = useNodeTypeStore(s => s.byTypeId)
  const syncConditionalEdgeLabels = useWorkflowStore(s => s.syncConditionalEdgeLabels)

  // Build ancestor node list with output fields (same pattern as UpstreamNodeReferences)
  const ancestors = useMemo(() => {
    return getAncestors(nodeId, edges)
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
  }, [nodeId, edges, nodes, outputParsers, byTypeId])

  // ---------------------------------------------------------------------------
  // Legacy format: config.expression is set
  // ---------------------------------------------------------------------------
  const legacyExpr = config['expression'] as string | undefined

  const handleMigrateToRules = useCallback(() => {
    const initial: ConditionalRule[] = [
      { label: 'rule_1', logic: 'AND', conditions: [emptyCondition()] },
    ]
    // Remove 'expression', set 'rules'
    const next = { ...config, rules: initial }
    delete next['expression']
    onChange(next)
  }, [config, onChange])

  const handleLegacyExprChange = useCallback((expr: string) => {
    onChange({ ...config, expression: expr })
  }, [config, onChange])

  if (legacyExpr !== undefined) {
    return (
      <div className="space-y-2">
        <div className="rounded-md border border-amber-700 bg-amber-900/30 px-3 py-2">
          <p className="text-xs font-semibold text-amber-400 mb-1">Legacy CEL expression</p>
          <p className="text-[10px] text-amber-300 mb-2">
            This node uses a raw CEL expression. It will continue to work as-is.
            Switch to the visual rule builder to use multiple branches.
          </p>
          <textarea
            value={legacyExpr}
            onChange={e => handleLegacyExprChange(e.target.value)}
            rows={2}
            className="w-full bg-gray-800 border border-gray-600 text-gray-100 text-xs font-mono rounded px-2 py-1.5 focus:outline-none focus:border-indigo-500 resize-none"
            placeholder='ctx["n1"]["status_code"] == 200'
          />
          <button
            onClick={handleMigrateToRules}
            className="mt-1.5 text-[10px] text-indigo-400 hover:text-indigo-300 transition-colors underline underline-offset-2"
          >
            Switch to visual rule builder (edge labels must be updated manually)
          </button>
        </div>
      </div>
    )
  }

  // ---------------------------------------------------------------------------
  // New format: config.rules
  // ---------------------------------------------------------------------------
  const rules = (config['rules'] as ConditionalRule[] | undefined) ?? []

  const setRules = useCallback((next: ConditionalRule[]) => {
    onChange({ ...config, rules: next })
    syncConditionalEdgeLabels(nodeId, next)
  }, [config, onChange, syncConditionalEdgeLabels, nodeId])

  // Duplicate label detection
  const labelCounts = rules.reduce<Record<string, number>>((acc, r) => {
    acc[r.label] = (acc[r.label] ?? 0) + 1
    return acc
  }, {})

  const updateRule = (i: number, r: ConditionalRule) => {
    setRules(rules.map((old, idx) => (idx === i ? r : old)))
  }
  const removeRule = (i: number) => setRules(rules.filter((_, idx) => idx !== i))
  const addRule    = () => setRules([...rules, emptyRule(rules.length)])
  const moveUp   = (i: number) => {
    if (i === 0) return
    const next = [...rules]
    ;[next[i - 1], next[i]] = [next[i], next[i - 1]]
    setRules(next)
  }
  const moveDown = (i: number) => {
    if (i === rules.length - 1) return
    const next = [...rules]
    ;[next[i], next[i + 1]] = [next[i + 1], next[i]]
    setRules(next)
  }

  const rulesError = fieldErrors?.['rules']

  return (
    <div className="space-y-2">
      {rulesError && (
        <div className="rounded-md bg-red-900/40 border border-red-700 px-2 py-1.5">
          <p className="text-xs text-red-300">{rulesError}</p>
        </div>
      )}

      {/* Rule cards */}
      {rules.map((rule, i) => (
        <RuleCard
          key={i}
          rule={rule}
          index={i}
          total={rules.length}
          labelError={
            rule.label === 'fallback'
              ? '"fallback" is reserved'
              : rule.label === ''
              ? 'Label is required'
              : (labelCounts[rule.label] ?? 0) > 1
              ? 'Duplicate label'
              : undefined
          }
          ancestors={ancestors}
          onChange={r => updateRule(i, r)}
          onRemove={() => removeRule(i)}
          onMoveUp={() => moveUp(i)}
          onMoveDown={() => moveDown(i)}
        />
      ))}

      {/* Add rule */}
      <button
        onClick={addRule}
        className="w-full rounded-md border border-dashed border-gray-600 py-1.5 text-xs text-gray-400 hover:border-indigo-500 hover:text-indigo-400 transition-colors"
      >
        + Add Rule
      </button>

      {/* Fallback chip */}
      <div className="flex items-center gap-2 rounded-md border border-gray-700 bg-gray-800/50 px-2 py-1.5">
        <span className="text-[10px] font-mono bg-gray-700 text-gray-300 px-1.5 py-0.5 rounded">
          fallback
        </span>
        <span className="text-[10px] text-gray-500">
          fires when no rule matches — connect a "fallback" edge from this node
        </span>
      </div>
    </div>
  )
}
