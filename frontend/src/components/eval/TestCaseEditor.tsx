import { useState } from 'react'
import type { TestCase, GraderDef, NodeMock, GraderType, GraderScope } from '../../api/types'
import { MockEditor } from './MockEditor'
import type { NodeOption } from './MockEditor'
import { GraderEditor } from './GraderEditor'

const inputCls =
  'w-full rounded-md bg-gray-900 border border-gray-600 px-3 py-1.5 text-sm text-gray-100 placeholder-gray-500 focus:outline-none focus:border-indigo-500'

const textareaCls =
  'w-full rounded-md bg-gray-900 border border-gray-600 px-3 py-1.5 text-sm text-gray-100 font-mono placeholder-gray-500 focus:outline-none focus:border-indigo-500 resize-y'

function newGrader(): GraderDef {
  return {
    id: crypto.randomUUID(),
    name: '',
    type: 'string_match' as GraderType,
    scope: 'workflow' as GraderScope,
    config: {},
  }
}

function newMock(): NodeMock {
  return { node_id: '', output: {} }
}

// Parse server-side field errors from paths like "graders.0.config.rubric"
// into a per-grader and per-mock map.
function parseFieldErrors(errs: { field?: string; message: string }[]) {
  const graders: Record<number, Record<string, string>> = {}
  const mocks: Record<number, Record<string, string>> = {}
  const general: string[] = []
  for (const e of errs) {
    const f = e.field ?? ''
    const gm = f.match(/^graders\.(\d+)\.(.+)$/)
    if (gm) {
      const idx = Number(gm[1])
      const key = gm[2].replace(/^config\./, '')
      if (!graders[idx]) graders[idx] = {}
      graders[idx][key] = e.message
      continue
    }
    const mm = f.match(/^mocks\.(\d+)\.(.+)$/)
    if (mm) {
      const idx = Number(mm[1])
      const key = mm[2]
      if (!mocks[idx]) mocks[idx] = {}
      mocks[idx][key] = e.message
      continue
    }
    general.push(f ? `${f}: ${e.message}` : e.message)
  }
  return { graders, mocks, general }
}

interface SchemaProperty {
  type?: string
  title?: string
  description?: string
}

interface InitialDataProps {
  schema: Record<string, unknown> | null
  value: Record<string, unknown>
  onChange: (v: Record<string, unknown>) => void
}

function InitialDataSection({ schema, value, onChange }: InitialDataProps) {
  const properties = schema
    ? ((schema.properties as Record<string, SchemaProperty> | undefined) ?? {})
    : {}
  const fieldNames = Object.keys(properties)

  const [rawText, setRawText] = useState(() =>
    Object.keys(value).length ? JSON.stringify(value, null, 2) : '{\n\n}'
  )
  const [jsonError, setJsonError] = useState<string | null>(null)

  if (fieldNames.length > 0) {
    return (
      <div className="space-y-2">
        {fieldNames.map(name => {
          const prop = properties[name]
          const label = prop.title ?? name
          const type = prop.type ?? 'string'
          const val = value[name]
          const setField = (v: unknown) => onChange({ ...value, [name]: v })

          return (
            <div key={name} className="space-y-1">
              <label className="block text-xs font-semibold text-gray-300">
                {label}
                <span className="ml-1.5 font-normal text-gray-500 font-mono text-[10px]">{type}</span>
              </label>
              {prop.description && <p className="text-xs text-gray-500">{prop.description}</p>}
              {type === 'boolean' ? (
                <label className="flex items-center gap-2 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={!!val}
                    onChange={e => setField(e.target.checked)}
                    className="rounded border-gray-600 bg-gray-900 text-indigo-500"
                  />
                  <span className="text-sm text-gray-300">{val ? 'true' : 'false'}</span>
                </label>
              ) : (type === 'number' || type === 'integer') ? (
                <input
                  type="number"
                  className={inputCls}
                  value={val !== undefined ? String(val) : ''}
                  onChange={e => setField(e.target.value === '' ? undefined : Number(e.target.value))}
                />
              ) : (
                <input
                  type="text"
                  className={inputCls}
                  value={val !== undefined ? String(val) : ''}
                  onChange={e => setField(e.target.value)}
                />
              )}
            </div>
          )
        })}
      </div>
    )
  }

  return (
    <div className="space-y-1">
      <textarea
        rows={4}
        className={textareaCls}
        value={rawText}
        onChange={e => {
          setRawText(e.target.value)
          try {
            const parsed = JSON.parse(e.target.value)
            if (typeof parsed === 'object' && !Array.isArray(parsed) && parsed !== null) {
              setJsonError(null)
              onChange(parsed as Record<string, unknown>)
            }
          } catch {
            setJsonError('Invalid JSON')
          }
        }}
        placeholder="{}"
      />
      {jsonError && <p className="text-xs text-red-400">{jsonError}</p>}
    </div>
  )
}

interface Props {
  testCase?: TestCase
  nodes: NodeOption[]
  initialDataSchema: Record<string, unknown> | null
  onSave: (data: Omit<TestCase, 'id' | 'suite_id' | 'position' | 'created_at' | 'updated_at'>) => Promise<void>
  onClose: () => void
  saving?: boolean
  serverErrors?: { field?: string; message: string }[]
}

export function TestCaseEditor({
  testCase,
  nodes,
  initialDataSchema,
  onSave,
  onClose,
  saving,
  serverErrors = [],
}: Props) {
  const [name, setName] = useState(testCase?.name ?? '')
  const [description, setDescription] = useState(testCase?.description ?? '')
  const [initialData, setInitialData] = useState<Record<string, unknown>>(
    testCase?.initial_data ?? {}
  )
  const [mocks, setMocks] = useState<NodeMock[]>(testCase?.mocks ?? [])
  const [graders, setGraders] = useState<GraderDef[]>(testCase?.graders ?? [])

  const { graders: graderErrors, mocks: mockErrors, general: generalErrors } =
    parseFieldErrors(serverErrors)

  const handleSave = async () => {
    await onSave({ name, description: description || undefined, initial_data: initialData, mocks, graders })
  }

  const updateGrader = (i: number, g: GraderDef) =>
    setGraders(prev => prev.map((x, idx) => idx === i ? g : x))

  const removeGrader = (i: number) =>
    setGraders(prev => prev.filter((_, idx) => idx !== i))

  const updateMock = (i: number, m: NodeMock) =>
    setMocks(prev => prev.map((x, idx) => idx === i ? m : x))

  const removeMock = (i: number) =>
    setMocks(prev => prev.filter((_, idx) => idx !== i))

  return (
    <div className="fixed inset-0 z-50 flex">
      {/* Backdrop */}
      <div className="flex-1 bg-black/50" onClick={onClose} />

      {/* Panel */}
      <div className="w-[640px] bg-gray-900 border-l border-gray-700 flex flex-col h-full shadow-2xl">
        {/* Header */}
        <div className="flex items-center justify-between px-4 py-3 border-b border-gray-700 flex-shrink-0">
          <h2 className="text-sm font-semibold text-gray-100">
            {testCase ? 'Edit Test Case' : 'New Test Case'}
          </h2>
          <button
            onClick={onClose}
            className="text-gray-500 hover:text-gray-300 transition-colors text-sm"
          >
            ✕
          </button>
        </div>

        {/* Body */}
        <div className="flex-1 overflow-y-auto p-4 space-y-6">
          {/* General server errors */}
          {generalErrors.length > 0 && (
            <div className="rounded-md bg-red-900/40 border border-red-700 px-3 py-2 space-y-1">
              {generalErrors.map((e, i) => (
                <p key={i} className="text-xs text-red-300">{e}</p>
              ))}
            </div>
          )}

          {/* Name & description */}
          <section className="space-y-3">
            <div className="space-y-1">
              <label className="block text-xs font-semibold text-gray-300">Name <span className="text-red-400">*</span></label>
              <input
                className={inputCls}
                placeholder="Happy path"
                value={name}
                onChange={e => setName(e.target.value)}
              />
            </div>
            <div className="space-y-1">
              <label className="block text-xs font-semibold text-gray-300">Description (optional)</label>
              <input
                className={inputCls}
                placeholder="What this test case verifies…"
                value={description}
                onChange={e => setDescription(e.target.value)}
              />
            </div>
          </section>

          {/* Initial data */}
          <section className="space-y-2">
            <h3 className="text-xs font-semibold text-gray-400 uppercase tracking-wide">Initial Data</h3>
            <p className="text-xs text-gray-500">Passed to the workflow as the run's initial data.</p>
            <InitialDataSection
              schema={initialDataSchema}
              value={initialData}
              onChange={setInitialData}
            />
          </section>

          {/* Mocks */}
          <section className="space-y-2">
            <div className="flex items-center justify-between">
              <h3 className="text-xs font-semibold text-gray-400 uppercase tracking-wide">
                Node Mocks <span className="text-gray-600 font-normal normal-case">({mocks.length})</span>
              </h3>
              <button
                type="button"
                onClick={() => setMocks(prev => [...prev, newMock()])}
                className="text-xs text-indigo-400 hover:text-indigo-300 transition-colors"
              >
                + Add Mock
              </button>
            </div>
            {mocks.length === 0 && (
              <p className="text-xs text-gray-600 italic">No mocks — all nodes will execute normally.</p>
            )}
            <div className="space-y-2">
              {mocks.map((m, i) => (
                <MockEditor
                  key={i}
                  mock={m}
                  nodes={nodes}
                  onChange={updated => updateMock(i, updated)}
                  onRemove={() => removeMock(i)}
                  nodeError={mockErrors[i]?.node_id}
                  outputError={mockErrors[i]?.output}
                />
              ))}
            </div>
          </section>

          {/* Graders */}
          <section className="space-y-2">
            <div className="flex items-center justify-between">
              <h3 className="text-xs font-semibold text-gray-400 uppercase tracking-wide">
                Graders <span className="text-gray-600 font-normal normal-case">({graders.length})</span>
              </h3>
              <button
                type="button"
                onClick={() => setGraders(prev => [...prev, newGrader()])}
                className="text-xs text-indigo-400 hover:text-indigo-300 transition-colors"
              >
                + Add Grader
              </button>
            </div>
            {graders.length === 0 && (
              <p className="text-xs text-gray-600 italic">No graders — test case passes if the workflow succeeds.</p>
            )}
            <div className="space-y-2">
              {graders.map((g, i) => (
                <GraderEditor
                  key={g.id}
                  grader={g}
                  nodes={nodes}
                  onChange={updated => updateGrader(i, updated)}
                  onRemove={() => removeGrader(i)}
                  fieldErrors={graderErrors[i]}
                />
              ))}
            </div>
          </section>
        </div>

        {/* Footer */}
        <div className="px-4 py-3 border-t border-gray-700 flex justify-end gap-2 flex-shrink-0">
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
            {saving ? 'Saving…' : 'Save'}
          </button>
        </div>
      </div>
    </div>
  )
}
