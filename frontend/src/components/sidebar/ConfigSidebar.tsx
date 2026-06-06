import { useRef } from 'react'
import { useWorkflowStore } from '../../stores/useWorkflowStore'
import { useNodeTypeStore } from '../../stores/useNodeTypeStore'
import { SchemaForm, getTemplateFields } from './SchemaForm'
import { TemplateVariablePicker } from './TemplateVariablePicker'
import { UpstreamNodeReferences } from './UpstreamNodeReferences'
import { OutputParserPanel } from './OutputParserPanel'
import { ConditionalRuleBuilder } from './ConditionalRuleBuilder'

export function ConfigSidebar() {
  const selectedNodeId = useWorkflowStore(s => s.selectedNodeId)
  const nodes = useWorkflowStore(s => s.nodes)
  const configs = useWorkflowStore(s => s.configs)
  const updateNodeConfig = useWorkflowStore(s => s.updateNodeConfig)
  const updateNodeLabel = useWorkflowStore(s => s.updateNodeLabel)
  const selectNode = useWorkflowStore(s => s.selectNode)
  const nodeErrors = useWorkflowStore(s => s.nodeErrors)
  const fieldErrors = useWorkflowStore(s => s.fieldErrors)
  const byTypeId = useNodeTypeStore(s => s.byTypeId)

  // Shared ref for tracking the last-focused template input
  const templateInputRef = useRef<HTMLInputElement | HTMLTextAreaElement | null>(null)

  if (!selectedNodeId) return null

  const rfNode = nodes.find(n => n.id === selectedNodeId)
  if (!rfNode) return null

  const { type_id, label } = rfNode.data
  const meta = byTypeId(type_id)
  const schema = meta?.input_schema ?? {}
  const config = configs[selectedNodeId] ?? {}
  const templateFields = meta ? getTemplateFields(meta.input_schema) : []
  const currentNodeErrors = nodeErrors[selectedNodeId]
  const currentFieldErrors = fieldErrors[selectedNodeId]

  return (
    <aside className="w-72 flex-shrink-0 border-l border-gray-700 bg-gray-800 flex flex-col overflow-hidden">
      {/* Header */}
      <div className="flex items-center justify-between px-3 py-2 border-b border-gray-700">
        <div className="min-w-0">
          <div className="text-xs font-mono text-gray-400 truncate">{type_id}</div>
          <div className="text-sm font-semibold text-gray-100 truncate">
            {meta?.display_name ?? type_id}
          </div>
        </div>
        <button
          onClick={() => selectNode(null)}
          className="ml-2 text-gray-500 hover:text-gray-300 transition-colors flex-shrink-0"
          title="Close"
        >
          ✕
        </button>
      </div>

      {/* Scrollable body */}
      <div className="flex-1 overflow-y-auto p-3 space-y-3">
        {/* Validation error banner */}
        {currentNodeErrors && currentNodeErrors.length > 0 && (
          <div className="rounded-md bg-red-900/40 border border-red-700 px-3 py-2">
            <p className="text-xs font-semibold text-red-400 mb-1">Save validation errors</p>
            <ul className="list-disc list-inside space-y-0.5">
              {currentNodeErrors.map((msg, i) => (
                <li key={i} className="text-xs text-red-300">{msg}</li>
              ))}
            </ul>
          </div>
        )}
        {/* Label */}
        <div>
          <label className="text-xs font-semibold uppercase tracking-wider text-gray-400 block mb-1">
            Label
          </label>
          <input
            type="text"
            value={label}
            onChange={e => updateNodeLabel(selectedNodeId, e.target.value)}
            className="
              w-full rounded-md bg-gray-700 border border-gray-600
              px-2 py-1.5 text-sm text-gray-100
              focus:outline-none focus:border-indigo-500
            "
          />
        </div>

        {/* Config form — conditional.branch gets a visual rule builder; all other nodes use SchemaForm */}
        {type_id === 'conditional.branch' ? (
          <div>
            <label className="text-xs font-semibold uppercase tracking-wider text-gray-400 block mb-1">
              Rules
            </label>
            <ConditionalRuleBuilder
              nodeId={selectedNodeId}
              config={config}
              onChange={data => updateNodeConfig(selectedNodeId, data)}
              fieldErrors={currentFieldErrors}
            />
          </div>
        ) : (
          meta && Object.keys(schema).length > 0 && (
            <div>
              <label className="text-xs font-semibold uppercase tracking-wider text-gray-400 block mb-1">
                Configuration
              </label>
              <SchemaForm
                nodeId={selectedNodeId}
                schema={schema}
                formData={config}
                onChange={data => updateNodeConfig(selectedNodeId, data)}
                focusRef={templateInputRef}
                fieldErrors={currentFieldErrors}
              />
            </div>
          )
        )}

        {/* Upstream node references — always visible when upstream nodes exist.
            Shows node IDs and CEL-ready field references for every node type,
            including conditional nodes whose expression field is not x-template. */}
        <UpstreamNodeReferences nodeId={selectedNodeId} />

        {/* Template variable picker — click-to-insert snippets for x-template fields */}
        {templateFields.length > 0 && (
          <TemplateVariablePicker
            nodeId={selectedNodeId}
            templateFields={templateFields}
          />
        )}

        {/* Output parsers */}
        <OutputParserPanel nodeId={selectedNodeId} typeId={type_id} />
      </div>
    </aside>
  )
}
