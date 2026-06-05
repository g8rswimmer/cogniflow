import { useMemo, useState } from 'react'
import { useWorkflowStore, getAncestors } from '../../stores/useWorkflowStore'
import { useNodeTypeStore } from '../../stores/useNodeTypeStore'

interface Props {
  nodeId: string
}

export function UpstreamNodeReferences({ nodeId }: Props) {
  const edges = useWorkflowStore(s => s.edges)
  const nodes = useWorkflowStore(s => s.nodes)
  const outputParsers = useWorkflowStore(s => s.outputParsers)
  const byTypeId = useNodeTypeStore(s => s.byTypeId)
  const [copied, setCopied] = useState<string | null>(null)

  const ancestors = useMemo(() => {
    return getAncestors(nodeId, edges)
      .map(id => {
        const rfNode = nodes.find(n => n.id === id)
        if (!rfNode) return null
        const meta = byTypeId(rfNode.data.type_id)
        const outputSchema = (meta?.output_schema ?? {}) as Record<string, unknown>
        const properties =
          (outputSchema.properties as Record<string, Record<string, unknown>> | undefined) ?? {}
        const schemaFields = Object.keys(properties)
        const parserFields = Object.keys(outputParsers[id] ?? {})
        const allFields = [...new Set([...schemaFields, ...parserFields])]
        return { id, label: (rfNode.data.label as string) || id, fields: allFields }
      })
      .filter(Boolean) as { id: string; label: string; fields: string[] }[]
  }, [nodeId, edges, nodes, outputParsers, byTypeId])

  if (ancestors.length === 0) return null

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text).catch(() => undefined)
    setCopied(text)
    setTimeout(() => setCopied(null), 1500)
  }

  return (
    <div className="rounded-md border border-dashed border-gray-600 bg-gray-800/50 p-2">
      <div className="text-xs font-semibold text-gray-400 mb-2 flex items-center gap-1">
        <span>↑</span> Upstream Nodes
        <span className="font-normal text-gray-600 ml-1">— IDs for CEL expressions</span>
      </div>

      <div className="space-y-2">
        {ancestors.map(node => (
          <div key={node.id} className="text-xs">
            {/* Label + copy button */}
            <div className="flex items-center gap-1 mb-0.5">
              <span className="text-gray-200 font-medium truncate flex-1">{node.label}</span>
              <button
                onClick={() => copyToClipboard(node.id)}
                className="flex-shrink-0 text-gray-500 hover:text-indigo-400 transition-colors"
                title={`Copy node ID: ${node.id}`}
              >
                {copied === node.id ? (
                  <span className="text-green-400">✓ copied</span>
                ) : (
                  <span className="underline underline-offset-2">copy id</span>
                )}
              </button>
            </div>

            {/* Node ID */}
            <div className="font-mono text-gray-500 truncate mb-0.5 text-[10px]">{node.id}</div>

            {/* CEL snippet */}
            <div className="font-mono text-gray-600 text-[10px] mb-1">
              {'ctx["'}{node.id}{'"]["field"]'}
            </div>

            {/* Output fields */}
            {node.fields.length > 0 && (
              <div className="flex flex-wrap gap-1">
                {node.fields.map(field => (
                  <button
                    key={field}
                    onClick={() => copyToClipboard(`ctx["${node.id}"]["${field}"]`)}
                    title={`Copy CEL reference: ctx["${node.id}"]["${field}"]`}
                    className="font-mono bg-gray-700 hover:bg-gray-600 text-gray-300 hover:text-indigo-300 rounded px-1.5 py-0.5 transition-colors"
                  >
                    {field}
                  </button>
                ))}
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  )
}
