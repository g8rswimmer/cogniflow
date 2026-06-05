import { memo } from 'react'
import { Handle, Position } from '@xyflow/react'
import type { NodeProps, Node } from '@xyflow/react'
import type { NodeData } from '../../stores/useWorkflowStore'
import { useRunStore } from '../../stores/useRunStore'

const categoryColors: Record<string, string> = {
  ai: 'bg-purple-700 border-purple-500',
  plugin: 'bg-amber-700 border-amber-500',
  trigger: 'bg-green-700 border-green-500',
  deterministic: 'bg-blue-700 border-blue-500',
}

const statusRing: Record<string, string> = {
  'node.pending': 'ring-gray-500',
  'node.running': 'ring-amber-400',
  'node.succeeded': 'ring-green-400',
  'node.failed': 'ring-red-500',
}

const statusDot: Record<string, string> = {
  'node.pending': 'bg-gray-500',
  'node.running': 'bg-amber-400 animate-pulse',
  'node.succeeded': 'bg-green-400',
  'node.failed': 'bg-red-500',
}

function getCategoryColor(typeId: string): string {
  const prefix = typeId.split('.')[0]
  if (prefix === 'llm' || prefix === 'embedding' || prefix === 'rag') return categoryColors.ai
  return categoryColors.deterministic
}

function CustomNode({ id, data, selected }: NodeProps<Node<NodeData>>) {
  const colorClass = getCategoryColor(data.type_id)
  const nodeStatus = useRunStore(s => s.nodeStatuses[id])
  const runStatus = useRunStore(s => s.runStatus)

  const showStatus = runStatus !== 'idle' && !!nodeStatus
  const ringClass = showStatus ? `ring-2 ${statusRing[nodeStatus.status] ?? 'ring-gray-500'}` : ''

  return (
    <div
      className={`
        min-w-[160px] rounded-lg border-2 shadow-lg transition-all
        ${selected ? 'ring-2 ring-indigo-400 ring-offset-1 ring-offset-gray-900' : ''}
        ${showStatus && !selected ? ringClass : ''}
        ${colorClass}
      `}
    >
      <Handle
        type="target"
        position={Position.Top}
        className="!bg-gray-300 !border-gray-500"
      />

      <div className="px-3 py-2 relative">
        {showStatus && (
          <span
            className={`absolute top-1 right-1 w-2.5 h-2.5 rounded-full ${statusDot[nodeStatus.status] ?? 'bg-gray-500'}`}
          />
        )}
        <div className="text-xs text-gray-300 font-mono truncate">{data.type_id}</div>
        <div className="text-sm text-white font-semibold mt-0.5 truncate">{data.label}</div>
      </div>

      <Handle
        type="source"
        position={Position.Bottom}
        className="!bg-gray-300 !border-gray-500"
      />
    </div>
  )
}

export default memo(CustomNode)
