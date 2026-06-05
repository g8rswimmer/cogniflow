import { useMemo, useRef, useCallback } from 'react'
import Form from '@rjsf/core'
import validator from '@rjsf/validator-ajv8'
import type { RJSFSchema, UiSchema, WidgetProps } from '@rjsf/utils'

// Module-level ref so TemplateVariablePicker can call insertSnippet() without prop-drilling.
// See lib/templateFocus.ts for the insertion logic.

function buildUiSchema(schema: Record<string, unknown>): UiSchema {
  const properties =
    (schema.properties as Record<string, Record<string, unknown>> | undefined) ?? {}
  const uiSchema: UiSchema = {}

  for (const [key, prop] of Object.entries(properties)) {
    if (prop['x-sensitive']) {
      uiSchema[key] = { 'ui:widget': 'password', 'ui:placeholder': '••••••••' }
    } else if (prop['x-template']) {
      uiSchema[key] = { 'ui:widget': 'TemplateTextWidget' }
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

interface Props {
  schema: Record<string, unknown>
  formData: Record<string, unknown>
  onChange: (data: Record<string, unknown>) => void
  focusRef: React.MutableRefObject<HTMLInputElement | HTMLTextAreaElement | null>
}

export function SchemaForm({ schema, formData, onChange, focusRef }: Props) {
  const uiSchema = useMemo(() => buildUiSchema(schema), [schema])
  const widgets = useMemo(() => ({ TemplateTextWidget: makeTemplateWidget(focusRef) }), [focusRef])

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
        validator={validator}
        widgets={widgets}
        onChange={handleChange}
        noHtml5Validate
        liveValidate={false}
      >
        {/* No submit button — save handled by Navbar */}
        <span />
      </Form>
    </div>
  )
}

export function getTemplateFields(schema: Record<string, unknown>): string[] {
  const properties =
    (schema.properties as Record<string, Record<string, unknown>> | undefined) ?? {}
  return Object.entries(properties)
    .filter(([, p]) => p['x-template'])
    .map(([k]) => k)
}
