import type { GraderType } from '../../api/types'

const inputCls =
  'w-full rounded-md bg-gray-900 border border-gray-600 px-3 py-1.5 text-sm text-gray-100 placeholder-gray-500 focus:outline-none focus:border-indigo-500'

const selectCls =
  'w-full rounded-md bg-gray-900 border border-gray-600 px-3 py-1.5 text-sm text-gray-100 focus:outline-none focus:border-indigo-500'

function Field({ label, children, error }: { label: string; children: React.ReactNode; error?: string }) {
  return (
    <div className="space-y-1">
      <label className="block text-xs font-semibold text-gray-300">{label}</label>
      {children}
      {error && <p className="text-xs text-red-400">{error}</p>}
    </div>
  )
}

// ---------------------------------------------------------------------------
// String Match
// ---------------------------------------------------------------------------

interface StringMatchProps {
  config: Record<string, unknown>
  onChange: (config: Record<string, unknown>) => void
  errors?: Record<string, string>
}

function StringMatchFields({ config, onChange, errors }: StringMatchProps) {
  const set = (k: string, v: unknown) => onChange({ ...config, [k]: v })
  return (
    <div className="space-y-3">
      <Field label="Field path" error={errors?.field_path}>
        <input
          className={inputCls}
          placeholder="n1.completion"
          value={(config.field_path as string) ?? ''}
          onChange={e => set('field_path', e.target.value)}
        />
      </Field>
      <Field label="Match type" error={errors?.match_type}>
        <select
          className={selectCls}
          value={(config.match_type as string) ?? 'contains'}
          onChange={e => set('match_type', e.target.value)}
        >
          <option value="exact">Exact</option>
          <option value="contains">Contains</option>
          <option value="regex">Regex</option>
        </select>
      </Field>
      <Field label="Expected value" error={errors?.expected_value}>
        <input
          className={inputCls}
          placeholder="expected text"
          value={(config.expected_value as string) ?? ''}
          onChange={e => set('expected_value', e.target.value)}
        />
      </Field>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Numeric Threshold
// ---------------------------------------------------------------------------

interface NumericProps {
  config: Record<string, unknown>
  onChange: (config: Record<string, unknown>) => void
  errors?: Record<string, string>
}

function NumericFields({ config, onChange, errors }: NumericProps) {
  const set = (k: string, v: unknown) => onChange({ ...config, [k]: v })
  return (
    <div className="space-y-3">
      <Field label="Field path" error={errors?.field_path}>
        <input
          className={inputCls}
          placeholder="n1.score"
          value={(config.field_path as string) ?? ''}
          onChange={e => set('field_path', e.target.value)}
        />
      </Field>
      <Field label="Operator" error={errors?.operator}>
        <select
          className={selectCls}
          value={(config.operator as string) ?? '>='}
          onChange={e => set('operator', e.target.value)}
        >
          {['==', '!=', '>', '>=', '<', '<='].map(op => (
            <option key={op} value={op}>{op}</option>
          ))}
        </select>
      </Field>
      <Field label="Threshold" error={errors?.threshold}>
        <input
          type="number"
          className={inputCls}
          placeholder="0"
          value={config.threshold !== undefined ? String(config.threshold) : ''}
          onChange={e => set('threshold', e.target.value === '' ? undefined : Number(e.target.value))}
        />
      </Field>
    </div>
  )
}

// ---------------------------------------------------------------------------
// LLM Judge
// ---------------------------------------------------------------------------

interface LLMJudgeProps {
  config: Record<string, unknown>
  onChange: (config: Record<string, unknown>) => void
  errors?: Record<string, string>
}

function LLMJudgeFields({ config, onChange, errors }: LLMJudgeProps) {
  const set = (k: string, v: unknown) => onChange({ ...config, [k]: v })
  return (
    <div className="space-y-3">
      <div className="grid grid-cols-2 gap-3">
        <Field label="Provider" error={errors?.provider}>
          <select
            className={selectCls}
            value={(config.provider as string) ?? 'anthropic'}
            onChange={e => set('provider', e.target.value)}
          >
            <option value="anthropic">Anthropic</option>
            <option value="openai">OpenAI</option>
          </select>
        </Field>
        <Field label="Model" error={errors?.model}>
          <input
            className={inputCls}
            placeholder="claude-haiku-4-5-20251001"
            value={(config.model as string) ?? ''}
            onChange={e => set('model', e.target.value)}
          />
        </Field>
      </div>
      <Field label="API key" error={errors?.api_key}>
        <input
          type="password"
          className={inputCls}
          placeholder="sk-… or masked if already set"
          value={(config.api_key as string) ?? ''}
          onChange={e => set('api_key', e.target.value)}
          autoComplete="off"
        />
      </Field>
      <Field label="Rubric" error={errors?.rubric}>
        <textarea
          rows={3}
          className={`${inputCls} resize-y font-mono`}
          placeholder="Describe what makes a response pass or fail…"
          value={(config.rubric as string) ?? ''}
          onChange={e => set('rubric', e.target.value)}
        />
      </Field>
      <Field label="Field path (optional — leave blank to evaluate full output)" error={errors?.field_path}>
        <input
          className={inputCls}
          placeholder="n1.completion"
          value={(config.field_path as string) ?? ''}
          onChange={e => set('field_path', e.target.value)}
        />
      </Field>
    </div>
  )
}

// ---------------------------------------------------------------------------
// JSON Schema
// ---------------------------------------------------------------------------

interface JSONSchemaProps {
  config: Record<string, unknown>
  onChange: (config: Record<string, unknown>) => void
  errors?: Record<string, string>
}

function JSONSchemaFields({ config, onChange, errors }: JSONSchemaProps) {
  const set = (k: string, v: unknown) => onChange({ ...config, [k]: v })
  const schemaText = config._schema_text as string | undefined
    ?? (config.schema ? JSON.stringify(config.schema, null, 2) : '')

  const handleSchemaChange = (text: string) => {
    try {
      const parsed = JSON.parse(text)
      onChange({ ...config, schema: parsed, _schema_text: text })
    } catch {
      onChange({ ...config, _schema_text: text })
    }
  }

  return (
    <div className="space-y-3">
      <Field label="Field path (optional)" error={errors?.field_path}>
        <input
          className={inputCls}
          placeholder="n1.output"
          value={(config.field_path as string) ?? ''}
          onChange={e => set('field_path', e.target.value)}
        />
      </Field>
      <Field label="JSON Schema" error={errors?.schema}>
        <textarea
          rows={6}
          className={`${inputCls} resize-y font-mono`}
          placeholder={'{\n  "type": "object",\n  "properties": {...}\n}'}
          value={schemaText}
          onChange={e => handleSchemaChange(e.target.value)}
        />
      </Field>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Checklist
// ---------------------------------------------------------------------------

interface ChecklistProps {
  config: Record<string, unknown>
  onChange: (config: Record<string, unknown>) => void
  errors?: Record<string, string>
}

function ChecklistFields({ config, onChange, errors }: ChecklistProps) {
  const set = (k: string, v: unknown) => onChange({ ...config, [k]: v })
  const criteria = (config.criteria as string[] | undefined) ?? []

  const updateCriterion = (i: number, val: string) => {
    const next = [...criteria]
    next[i] = val
    set('criteria', next)
  }

  const addCriterion = () => set('criteria', [...criteria, ''])

  const removeCriterion = (i: number) => {
    const next = criteria.filter((_, idx) => idx !== i)
    set('criteria', next)
  }

  return (
    <div className="space-y-3">
      <div className="grid grid-cols-2 gap-3">
        <Field label="Provider" error={errors?.provider}>
          <select
            className={selectCls}
            value={(config.provider as string) ?? 'anthropic'}
            onChange={e => set('provider', e.target.value)}
          >
            <option value="anthropic">Anthropic</option>
            <option value="openai">OpenAI</option>
          </select>
        </Field>
        <Field label="Model" error={errors?.model}>
          <input
            className={inputCls}
            placeholder="claude-haiku-4-5-20251001"
            value={(config.model as string) ?? ''}
            onChange={e => set('model', e.target.value)}
          />
        </Field>
      </div>
      <Field label="API key" error={errors?.api_key}>
        <input
          type="password"
          className={inputCls}
          placeholder="sk-… or masked if already set"
          value={(config.api_key as string) ?? ''}
          onChange={e => set('api_key', e.target.value)}
          autoComplete="off"
        />
      </Field>
      <div className="space-y-1">
        <label className="block text-xs font-semibold text-gray-300">Criteria</label>
        {errors?.criteria && <p className="text-xs text-red-400">{errors.criteria}</p>}
        <div className="space-y-2">
          {criteria.map((c, i) => (
            <div key={i} className="flex gap-2">
              <input
                className={`${inputCls} flex-1`}
                placeholder={`Criterion ${i + 1}…`}
                value={c}
                onChange={e => updateCriterion(i, e.target.value)}
              />
              <button
                type="button"
                onClick={() => removeCriterion(i)}
                className="text-gray-500 hover:text-red-400 transition-colors px-1 text-sm"
                title="Remove"
              >
                ✕
              </button>
            </div>
          ))}
        </div>
        <button
          type="button"
          onClick={addCriterion}
          className="mt-1 text-xs text-indigo-400 hover:text-indigo-300 transition-colors"
        >
          + Add Criterion
        </button>
      </div>
      <Field label="Pass threshold (0.0–1.0)" error={errors?.pass_threshold}>
        <input
          type="number"
          step="0.1"
          min="0"
          max="1"
          className={inputCls}
          placeholder="0.8"
          value={config.pass_threshold !== undefined ? String(config.pass_threshold) : ''}
          onChange={e => set('pass_threshold', e.target.value === '' ? undefined : Number(e.target.value))}
        />
      </Field>
      <Field label="Field path (optional)" error={errors?.field_path}>
        <input
          className={inputCls}
          placeholder="n1.completion"
          value={(config.field_path as string) ?? ''}
          onChange={e => set('field_path', e.target.value)}
        />
      </Field>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Dispatcher
// ---------------------------------------------------------------------------

interface Props {
  type: GraderType
  config: Record<string, unknown>
  onChange: (config: Record<string, unknown>) => void
  errors?: Record<string, string>
}

export function GraderTypeFields({ type, config, onChange, errors }: Props) {
  switch (type) {
    case 'string_match':
      return <StringMatchFields config={config} onChange={onChange} errors={errors} />
    case 'numeric_threshold':
      return <NumericFields config={config} onChange={onChange} errors={errors} />
    case 'llm_judge':
      return <LLMJudgeFields config={config} onChange={onChange} errors={errors} />
    case 'json_schema':
      return <JSONSchemaFields config={config} onChange={onChange} errors={errors} />
    case 'checklist':
      return <ChecklistFields config={config} onChange={onChange} errors={errors} />
  }
}
