import { useState, useMemo } from 'react'
import { useWorkflowStore } from '../../stores/useWorkflowStore'

type FieldType = 'string' | 'number' | 'boolean' | 'object' | 'array'

interface SchemaField {
  name: string
  type: FieldType
  description: string
}

const FIELD_TYPES: FieldType[] = ['string', 'number', 'boolean', 'object', 'array']

function schemaToFields(schema: Record<string, unknown> | null): SchemaField[] {
  if (!schema) return []
  const props = (schema.properties as Record<string, { type?: string; description?: string }> | undefined) ?? {}
  return Object.entries(props).map(([name, def]) => ({
    name,
    type: (def.type ?? 'string') as FieldType,
    description: def.description ?? '',
  }))
}

function fieldsToSchema(fields: SchemaField[]): Record<string, unknown> | null {
  if (fields.length === 0) return null
  return {
    type: 'object',
    properties: Object.fromEntries(
      fields.map(f => [
        f.name,
        { type: f.type, ...(f.description ? { description: f.description } : {}) },
      ]),
    ),
  }
}

const inputCls = `
  w-full rounded-md bg-gray-700 border border-gray-600
  px-2 py-1 text-sm text-gray-100
  focus:outline-none focus:border-indigo-500
`.trim()

const selectCls = `
  rounded-md bg-gray-700 border border-gray-600
  px-2 py-1 text-sm text-gray-100
  focus:outline-none focus:border-indigo-500
`.trim()

export function InitialDataSchemaEditor({ hideHeading = false }: { hideHeading?: boolean }) {
  const initialDataSchema = useWorkflowStore(s => s.initialDataSchema)
  const setInitialDataSchema = useWorkflowStore(s => s.setInitialDataSchema)

  const fields = useMemo(() => schemaToFields(initialDataSchema), [initialDataSchema])

  const [draftName, setDraftName] = useState('')
  const [draftType, setDraftType] = useState<FieldType>('string')
  const [draftDesc, setDraftDesc] = useState('')
  const [nameError, setNameError] = useState('')

  const updateFields = (next: SchemaField[]) => {
    setInitialDataSchema(fieldsToSchema(next))
  }

  const handleDelete = (name: string) => {
    updateFields(fields.filter(f => f.name !== name))
  }

  const handleTypeChange = (name: string, type: FieldType) => {
    updateFields(fields.map(f => f.name === name ? { ...f, type } : f))
  }

  const handleDescChange = (name: string, description: string) => {
    updateFields(fields.map(f => f.name === name ? { ...f, description } : f))
  }

  const handleAdd = () => {
    const trimmed = draftName.trim()
    if (!trimmed) {
      setNameError('Name is required')
      return
    }
    if (!/^[a-zA-Z_][a-zA-Z0-9_]*$/.test(trimmed)) {
      setNameError('Must start with a letter or underscore; only letters, digits, and underscores allowed')
      return
    }
    if (fields.some(f => f.name === trimmed)) {
      setNameError('Field name already exists')
      return
    }
    updateFields([...fields, { name: trimmed, type: draftType, description: draftDesc }])
    setDraftName('')
    setDraftType('string')
    setDraftDesc('')
    setNameError('')
  }

  return (
    <div className="space-y-3">
      {!hideHeading && (
        <div className="text-xs font-semibold uppercase tracking-wider text-gray-400">
          Workflow Inputs
        </div>
      )}
      <p className="text-xs text-gray-500">
        Declare the fields your workflow expects in its initial data.
        These appear as individual chips in the template variable picker
        (e.g.&nbsp;<span className="font-mono text-indigo-300">{'{{._initial.customer_id}}'}</span>).
      </p>

      {/* Existing fields */}
      {fields.length > 0 && (
        <div className="space-y-2">
          {fields.map(field => (
            <div
              key={field.name}
              className="rounded-md border border-gray-700 bg-gray-900/40 p-2 space-y-1.5"
            >
              <div className="flex items-center gap-2">
                <span className="flex-1 text-sm font-mono text-gray-100 truncate">{field.name}</span>
                <select
                  value={field.type}
                  onChange={e => handleTypeChange(field.name, e.target.value as FieldType)}
                  className={selectCls}
                  aria-label="Field type"
                >
                  {FIELD_TYPES.map(t => (
                    <option key={t} value={t}>{t}</option>
                  ))}
                </select>
                <button
                  onClick={() => handleDelete(field.name)}
                  className="text-gray-500 hover:text-red-400 transition-colors flex-shrink-0 text-sm"
                  title="Remove field"
                >
                  ✕
                </button>
              </div>
              <input
                type="text"
                value={field.description}
                onChange={e => handleDescChange(field.name, e.target.value)}
                placeholder="Description (optional)"
                className={inputCls}
              />
            </div>
          ))}
        </div>
      )}

      {/* Add field form */}
      <div className="rounded-md border border-dashed border-gray-600 bg-gray-800/50 p-2 space-y-2">
        <div className="text-xs text-gray-500 font-semibold">Add field</div>
        <div className="flex gap-2">
          <input
            type="text"
            value={draftName}
            onChange={e => { setDraftName(e.target.value); setNameError('') }}
            onKeyDown={e => { if (e.key === 'Enter') handleAdd() }}
            placeholder="field_name"
            className={`${inputCls} flex-1 min-w-0 font-mono`}
          />
          <select
            value={draftType}
            onChange={e => setDraftType(e.target.value as FieldType)}
            className={selectCls}
            aria-label="New field type"
          >
            {FIELD_TYPES.map(t => (
              <option key={t} value={t}>{t}</option>
            ))}
          </select>
        </div>
        {nameError && (
          <p className="text-xs text-red-400">{nameError}</p>
        )}
        <input
          type="text"
          value={draftDesc}
          onChange={e => setDraftDesc(e.target.value)}
          placeholder="Description (optional)"
          className={inputCls}
        />
        <div className="flex justify-end">
          <button
            onClick={handleAdd}
            className="
              rounded-md bg-indigo-600 hover:bg-indigo-500
              px-3 py-1 text-xs text-white font-semibold
              transition-colors
            "
          >
            Add Field
          </button>
        </div>
      </div>
    </div>
  )
}
