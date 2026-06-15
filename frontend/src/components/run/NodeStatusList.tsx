import { useState } from 'react'
import type { NodeRunStatus } from '../../stores/useRunStore'
import { nodeStatusDot, nodeStatusLabel } from '../../lib/nodeStatus'

export interface NodeEntry {
  nodeId: string
  label: string
  status?: NodeRunStatus
}

interface Props {
  entries: NodeEntry[]
}

export function NodeStatusList({ entries }: Props) {
  const [expanded, setExpanded] = useState<Record<string, boolean>>({})

  const toggle = (id: string) =>
    setExpanded(e => ({ ...e, [id]: !e[id] }))

  return (
    <div className="space-y-1">
      {entries.map(({ nodeId, label, status }) => {
        const hasDetail = !!(status?.output || status?.error)
        return (
          <div
            key={nodeId}
            className="rounded-md bg-gray-800 border border-gray-700 overflow-hidden"
          >
            <button
              onClick={() => hasDetail && toggle(nodeId)}
              className={`w-full flex items-center gap-2 px-3 py-2 text-left transition-colors ${hasDetail ? 'hover:bg-gray-700 cursor-pointer' : 'cursor-default'}`}
            >
              <span
                className={`w-2 h-2 rounded-full flex-shrink-0 ${status ? (nodeStatusDot[status.status] ?? 'bg-gray-500') : 'bg-gray-600'}`}
              />
              <span className="text-sm text-gray-100 font-medium flex-1 truncate">{label}</span>
              {status && (
                <span className="text-xs text-gray-400 flex-shrink-0">
                  {nodeStatusLabel[status.status] ?? status.status}
                </span>
              )}
              {hasDetail && (
                <span className="text-xs text-gray-500 flex-shrink-0">
                  {expanded[nodeId] ? '▲' : '▼'}
                </span>
              )}
            </button>

            {expanded[nodeId] && hasDetail && (
              <div className="px-3 pb-2 border-t border-gray-700">
                {status?.error && (
                  <pre className="text-xs text-red-400 mt-2 whitespace-pre-wrap break-all">
                    {status.error}
                  </pre>
                )}
                {status?.output && (
                  <pre className="text-xs text-green-300 mt-2 whitespace-pre-wrap break-all overflow-x-auto max-h-40">
                    {JSON.stringify(status.output, null, 2)}
                  </pre>
                )}
              </div>
            )}
          </div>
        )
      })}
    </div>
  )
}
