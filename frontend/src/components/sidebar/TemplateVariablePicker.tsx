import { useMemo } from 'react'
import { useWorkflowStore, getAncestors } from '../../stores/useWorkflowStore'
import { useNodeTypeStore } from '../../stores/useNodeTypeStore'
import { insertSnippet } from '../../lib/templateFocus'

interface Props {
  nodeId: string
  templateFields: string[]
}

export function TemplateVariablePicker({ nodeId, templateFields }: Props) {
  const edges = useWorkflowStore(s => s.edges)
  const nodes = useWorkflowStore(s => s.nodes)
  const outputParsers = useWorkflowStore(s => s.outputParsers)
  const byTypeId = useNodeTypeStore(s => s.byTypeId)

  const ancestorItems = useMemo(() => {
    const ancestorIds = getAncestors(nodeId, edges)
    return ancestorIds.map(id => {
      const rfNode = nodes.find(n => n.id === id)
      if (!rfNode) return null

      const meta = byTypeId(rfNode.data.type_id)
      const outputSchema = (meta?.output_schema ?? {}) as Record<string, unknown>
      const properties =
        (outputSchema.properties as Record<string, Record<string, unknown>> | undefined) ?? {}

      const schemaFields = Object.keys(properties)
      const parserFields = Object.keys(outputParsers[id] ?? {})
      const allFields = [...new Set([...schemaFields, ...parserFields])]

      return { id, label: rfNode.data.label || id, fields: allFields }
    }).filter(Boolean) as { id: string; label: string; fields: string[] }[]
  }, [nodeId, edges, nodes, outputParsers, byTypeId])

  // Offer ._initial fields derived from the workflow's declared initial data schema.
  // Falls back to showing the bare {{._initial}} chip when no schema is defined.
  const initialDataSchema = useWorkflowStore(s => s.initialDataSchema)
  const initialFields = useMemo(() => {
    const props = (initialDataSchema?.properties as Record<string, unknown> | undefined) ?? {}
    return Object.keys(props)
  }, [initialDataSchema])

  if (templateFields.length === 0) return null

  const handleClick = (snippet: string, e: React.MouseEvent) => {
    e.preventDefault()
    insertSnippet(snippet)
  }

  return (
    <div className="mt-2 rounded-md border border-dashed border-gray-600 bg-gray-800/50 p-2">
      <div className="text-xs font-semibold text-gray-400 mb-1.5 flex items-center gap-1">
        <span>⚡</span> Template Variables
      </div>

      {/* ._initial */}
      <div className="mb-2">
        <div className="text-xs text-gray-500 mb-1">Initial data</div>
        <div className="flex flex-wrap gap-1">
          {initialFields.length === 0 ? (
            // No schema defined — show the bare ._initial chip (backward compatible)
            <button
              onMouseDown={e => handleClick('{{._initial}}', e)}
              className="text-xs bg-gray-700 hover:bg-gray-600 text-indigo-300 rounded px-1.5 py-0.5 font-mono transition-colors"
            >
              {'{{._initial}}'}
            </button>
          ) : (
            // Schema defined — show one chip per declared field
            initialFields.map(field => {
              const snippet = `{{._initial.${field}}}`
              return (
                <button
                  key={field}
                  onMouseDown={e => handleClick(snippet, e)}
                  className="text-xs bg-gray-700 hover:bg-gray-600 text-indigo-300 rounded px-1.5 py-0.5 font-mono transition-colors"
                >
                  {snippet}
                </button>
              )
            })
          )}
        </div>
      </div>

      {ancestorItems.length === 0 && (
        <p className="text-xs text-gray-500 italic">No upstream nodes connected</p>
      )}

      {ancestorItems.map(item => (
        <div key={item.id} className="mb-2">
          <div className="text-xs text-gray-500 mb-1 truncate">
            {item.label} <span className="font-mono text-gray-600">({item.id})</span>
          </div>
          {item.fields.length === 0 ? (
            <p className="text-xs text-gray-600 italic">No declared output fields</p>
          ) : (
            <div className="flex flex-wrap gap-1">
              {item.fields.map(field => {
                const snippet = `{{.${item.id}.${field}}}`
                return (
                  <button
                    key={field}
                    onMouseDown={e => handleClick(snippet, e)}
                    className="text-xs bg-gray-700 hover:bg-gray-600 text-indigo-300 rounded px-1.5 py-0.5 font-mono transition-colors"
                  >
                    {snippet}
                  </button>
                )
              })}
            </div>
          )}
        </div>
      ))}
    </div>
  )
}
