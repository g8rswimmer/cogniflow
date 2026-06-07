import { useMemo, useRef, useCallback, useState } from 'react'
import Form from '@rjsf/core'
import validator from '@rjsf/validator-ajv8'
import type { RJSFSchema, UiSchema, WidgetProps } from '@rjsf/utils'
import { useWorkflowStore, getAncestors } from '../../stores/useWorkflowStore'
import { useNodeTypeStore } from '../../stores/useNodeTypeStore'
import { insertSnippet } from '../../lib/templateFocus'

function EyeIcon({ open }: { open: boolean }) {
  return open ? (
    <svg xmlns="http://www.w3.org/2000/svg" className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
      <path strokeLinecap="round" strokeLinejoin="round" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
      <path strokeLinecap="round" strokeLinejoin="round" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" />
    </svg>
  ) : (
    <svg xmlns="http://www.w3.org/2000/svg" className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
      <path strokeLinecap="round" strokeLinejoin="round" d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.88 9.88l-3.29-3.29m7.532 7.532l3.29 3.29M3 3l3.59 3.59m0 0A9.953 9.953 0 0112 5c4.478 0 8.268 2.943 9.543 7a10.025 10.025 0 01-4.132 5.411m0 0L21 21" />
    </svg>
  )
}

function SensitiveWidget(props: WidgetProps) {
  const { id, value, onChange, disabled, readonly, placeholder } = props
  const [show, setShow] = useState(false)

  return (
    <div className="relative flex items-center">
      <input
        id={id}
        type={show ? 'text' : 'password'}
        value={value ?? ''}
        disabled={disabled || readonly}
        placeholder={placeholder ?? '••••••••'}
        onChange={e => onChange(e.target.value === '' ? undefined : e.target.value)}
        className="
          w-full rounded-md bg-gray-700 border border-gray-600
          px-3 py-1.5 pr-9 text-sm text-gray-100 placeholder-gray-500
          focus:outline-none focus:border-indigo-500 font-mono
        "
      />
      <button
        type="button"
        onClick={() => setShow(v => !v)}
        title={show ? 'Hide' : 'Show'}
        className="absolute right-2 text-gray-400 hover:text-gray-200 transition-colors"
        tabIndex={-1}
      >
        <EyeIcon open={show} />
      </button>
    </div>
  )
}

function buildUiSchema(schema: Record<string, unknown>): UiSchema {
  const properties =
    (schema.properties as Record<string, Record<string, unknown>> | undefined) ?? {}
  const uiSchema: UiSchema = {}

  for (const [key, prop] of Object.entries(properties)) {
    if (prop['x-sensitive']) {
      uiSchema[key] = { 'ui:widget': 'SensitiveWidget' }
    } else if (prop['x-textarea'] && prop['x-template']) {
      uiSchema[key] = { 'ui:widget': 'TextareaTemplateWidget' }
    } else if (prop['x-template']) {
      uiSchema[key] = { 'ui:widget': 'TemplateTextWidget' }
    } else if (prop['x-textarea']) {
      uiSchema[key] = { 'ui:widget': 'textarea' }
    }
  }

  // Suppress the default submit button — we use our own Save button.
  uiSchema['ui:submitButtonOptions'] = { norender: true }

  return uiSchema
}

function makeTemplateWidget(
  onFocusRef: React.MutableRefObject<HTMLInputElement | HTMLTextAreaElement | null>,
) {
  return function TemplateTextWidget(props: WidgetProps) {
    const { id, value, onChange, onFocus: rjsfFocus, onBlur: rjsfBlur, disabled, readonly, placeholder } =
      props
    const inputRef = useRef<HTMLInputElement>(null)

    return (
      <input
        ref={inputRef}
        id={id}
        type="text"
        value={value ?? ''}
        disabled={disabled || readonly}
        placeholder={placeholder}
        onChange={e => onChange(e.target.value === '' ? undefined : e.target.value)}
        onFocus={() => {
          onFocusRef.current = inputRef.current
          rjsfFocus(id, value)
        }}
        onBlur={() => {
          // Keep the ref alive briefly so an onMouseDown chip click can still fire.
          setTimeout(() => {
            if (onFocusRef.current === inputRef.current) onFocusRef.current = null
          }, 200)
          rjsfBlur(id, value)
        }}
        className="
          w-full rounded-md bg-gray-700 border border-gray-600
          px-3 py-1.5 text-sm text-gray-100 placeholder-gray-400
          focus:outline-none focus:border-indigo-500
        "
      />
    )
  }
}

// ---------------------------------------------------------------------------
// TextareaTemplateWidget — module-level stable component so RJSF's function-
// equality check never prevents it from seeing a fresh nodeId. nodeId is read
// from RJSF formContext (props.registry.formContext) instead of a factory
// closure, so it is always current and correctly included in useMemo deps.
// ---------------------------------------------------------------------------

const selectCls =
  'appearance-none bg-gray-700 border border-gray-600 text-gray-100 text-xs rounded px-2 py-1.5 focus:outline-none focus:border-indigo-500 cursor-pointer'

function TextareaTemplateWidget(props: WidgetProps) {
  const { id, value, onChange, onFocus: rjsfFocus, onBlur: rjsfBlur, disabled, readonly, placeholder, registry } =
    props
  const nodeId = (registry.formContext as { nodeId?: string }).nodeId ?? ''

  const edges         = useWorkflowStore(s => s.edges)
  const nodes         = useWorkflowStore(s => s.nodes)
  const outputParsers = useWorkflowStore(s => s.outputParsers)
  const byTypeId      = useNodeTypeStore(s => s.byTypeId)

  const ancestors = useMemo(() => {
    return getAncestors(nodeId, edges)
      .map(ancestorId => {
        const rfNode = nodes.find(n => n.id === ancestorId)
        if (!rfNode) return null
        const meta = byTypeId(rfNode.data.type_id)
        const schemaProps =
          ((meta?.output_schema as Record<string, unknown> | undefined)?.properties as
            Record<string, unknown> | undefined) ?? {}
        const schemaFields = Object.keys(schemaProps)
        const parserFields = Object.keys(outputParsers[ancestorId] ?? {})
        return {
          id: ancestorId,
          label: (rfNode.data.label as string) || ancestorId,
          fields: [...new Set([...schemaFields, ...parserFields])],
        }
      })
      .filter(Boolean) as { id: string; label: string; fields: string[] }[]
  }, [nodeId, edges, nodes, outputParsers, byTypeId])

  // Single field state covers both _initial (free-text) and node-field (select) branches.
  const [pickerNodeId, setPickerNodeId] = useState('')
  const [pickerField, setPickerField]   = useState('')

  // Reset picker state during render when the selected node changes (avoids an
  // effect-triggered re-render; this is the React-recommended pattern for
  // "adjusting state when a prop changes").
  const [prevNodeId, setPrevNodeId] = useState(nodeId)
  if (nodeId !== prevNodeId) {
    setPrevNodeId(nodeId)
    setPickerNodeId('')
    setPickerField('')
  }

  const fieldsForPickerNode = useMemo(() => {
    if (!pickerNodeId || pickerNodeId === '_initial') return []
    return ancestors.find(a => a.id === pickerNodeId)?.fields ?? []
  }, [ancestors, pickerNodeId])

  const insertDisabled = !pickerNodeId || pickerField.trim() === ''

  return (
    <div className="space-y-1.5">
      <textarea
        id={id}
        rows={5}
        value={value ?? ''}
        disabled={disabled || readonly}
        placeholder={placeholder}
        onChange={e => onChange(e.target.value === '' ? undefined : e.target.value)}
        onFocus={() => rjsfFocus(id, value)}
        onBlur={() => rjsfBlur(id, value)}
        className="
          w-full rounded-md bg-gray-700 border border-gray-600
          px-3 py-1.5 text-sm text-gray-100 placeholder-gray-400
          focus:outline-none focus:border-indigo-500 resize-none font-mono leading-relaxed
        "
      />

      {/* Inline variable picker */}
      <div className="flex items-center gap-1.5 flex-wrap">
        {/* Node selector */}
        <select
          value={pickerNodeId}
          onChange={e => { setPickerNodeId(e.target.value); setPickerField('') }}
          className={`${selectCls} flex-1 min-w-0`}
          aria-label="Select upstream node"
        >
          <option value="">— node —</option>
          <option value="_initial">Initial Data</option>
          {ancestors.map(a => (
            <option key={a.id} value={a.id}>{a.label}</option>
          ))}
        </select>

        {/* Field selector / free-text for _initial */}
        {pickerNodeId === '_initial' ? (
          <input
            type="text"
            value={pickerField}
            onChange={e => setPickerField(e.target.value)}
            placeholder="field name"
            className="flex-1 min-w-0 bg-gray-700 border border-gray-600 text-gray-100 text-xs rounded px-2 py-1.5 focus:outline-none focus:border-indigo-500"
            aria-label="Initial data field name"
          />
        ) : (
          <select
            value={pickerField}
            onChange={e => setPickerField(e.target.value)}
            disabled={!pickerNodeId}
            className={`${selectCls} flex-1 min-w-0`}
            aria-label="Select field"
          >
            <option value="">— field —</option>
            {fieldsForPickerNode.map(f => (
              <option key={f} value={f}>{f}</option>
            ))}
          </select>
        )}

        {/* onMouseDown+preventDefault keeps the textarea focused so insertSnippet
            reads the correct document.activeElement and cursor position. */}
        <button
          type="button"
          onMouseDown={e => {
            if (insertDisabled) return
            e.preventDefault()
            const snippet = pickerNodeId === '_initial'
              ? `{{._initial.${pickerField}}}`
              : `{{.${pickerNodeId}.${pickerField}}}`
            insertSnippet(snippet)
          }}
          disabled={insertDisabled}
          className="flex-shrink-0 text-xs bg-indigo-700 hover:bg-indigo-600 disabled:opacity-30 disabled:cursor-not-allowed text-white rounded px-2 py-1.5 transition-colors"
        >
          Insert
        </button>
      </div>
    </div>
  )
}

interface Props {
  nodeId: string
  schema: Record<string, unknown>
  formData: Record<string, unknown>
  onChange: (data: Record<string, unknown>) => void
  focusRef: React.MutableRefObject<HTMLInputElement | HTMLTextAreaElement | null>
  fieldErrors?: Record<string, string>
}

export function SchemaForm({ nodeId, schema, formData, onChange, focusRef, fieldErrors }: Props) {
  const uiSchema = useMemo(() => buildUiSchema(schema), [schema])
  // TextareaTemplateWidget is module-level (stable reference); no nodeId dep needed.
  const widgets = useMemo(() => ({
    TemplateTextWidget: makeTemplateWidget(focusRef),
    TextareaTemplateWidget,
    SensitiveWidget,
  }), [focusRef])

  const extraErrors = useMemo(() => {
    if (!fieldErrors || Object.keys(fieldErrors).length === 0) return undefined
    return Object.fromEntries(
      Object.entries(fieldErrors).map(([field, msg]) => [field, { __errors: [msg] }])
    )
  }, [fieldErrors])

  // Track the last JSON snapshot we propagated upward. RJSF fires onChange on
  // mount with the initial formData, which would cause an infinite Zustand loop
  // (onChange → setState → re-render → new formData ref → onChange → …).
  const lastSentRef = useRef<string>('')

  const handleChange = useCallback(({ formData: fd }: { formData?: Record<string, unknown> }) => {
    if (fd === undefined) return
    const snapshot = JSON.stringify(fd)
    if (snapshot === lastSentRef.current) return
    lastSentRef.current = snapshot
    onChange(fd)
  }, [onChange])

  return (
    <div className="rjsf-cogniflow">
      <Form
        schema={schema as RJSFSchema}
        uiSchema={uiSchema}
        formData={formData}
        formContext={{ nodeId }}
        validator={validator}
        widgets={widgets}
        onChange={handleChange}
        extraErrors={extraErrors}
        noHtml5Validate
        liveValidate={false}
      >
        {/* No submit button — save handled by Navbar */}
        <span />
      </Form>
    </div>
  )
}

