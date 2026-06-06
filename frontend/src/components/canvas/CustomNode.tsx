import { memo } from 'react'
import { Handle, Position } from '@xyflow/react'
import type { NodeProps, Node } from '@xyflow/react'
import type { NodeData } from '../../stores/useWorkflowStore'
import { useWorkflowStore } from '../../stores/useWorkflowStore'
import { useRunStore } from '../../stores/useRunStore'
import { nodeStatusDot, nodeStatusRing } from '../../lib/nodeStatus'

const categoryColors: Record<string, string> = {
  ai: 'bg-purple-700 border-purple-500',
  plugin: 'bg-amber-700 border-amber-500',
  trigger: 'bg-green-700 border-green-500',
  deterministic: 'bg-blue-700 border-blue-500',
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
  const nodeErrors = useWorkflowStore(s => s.nodeErrors[id])

  const hasErrors = !!nodeErrors && nodeErrors.length > 0
  const showStatus = runStatus !== 'idle' && !!nodeStatus

  // Run-status ring takes priority over error ring during an active run.
  const ringClass = showStatus
    ? `ring-2 ${nodeStatusRing[nodeStatus.status] ?? 'ring-gray-500'}`
    : hasErrors
      ? 'ring-2 ring-red-500'
      : ''

  const errorTooltip = hasErrors ? nodeErrors.join('\n') : undefined

  return (
    <div
      className={`
        min-w-[160px] rounded-lg border-2 shadow-lg transition-all
        ${selected ? 'ring-2 ring-indigo-400 ring-offset-1 ring-offset-gray-900' : ''}
        ${!selected ? ringClass : ''}
        ${colorClass}
      `}
    >
      <Handle
        type="target"
        position={Position.Top}
        className="!bg-gray-300 !border-gray-500"
      />

      <div className="px-3 py-2 relative">
        {/* Run-status dot */}
        {showStatus && (
          <span
            className={`absolute top-1 right-1 w-2.5 h-2.5 rounded-full ${nodeStatusDot[nodeStatus.status] ?? 'bg-gray-500'}`}
          />
        )}
        {/* Save-error badge — only shown when no run is active */}
        {hasErrors && !showStatus && (
          <span
            className="absolute top-1 right-1 w-4 h-4 rounded-full bg-red-500 flex items-center justify-center text-white text-[9px] font-bold cursor-default"
            title={errorTooltip}
          >
            !
          </span>
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
