import { Link } from 'react-router-dom'
import { useWorkflowStore } from '../../stores/useWorkflowStore'
import { useRunStore } from '../../stores/useRunStore'
import { NodeStatusList } from './NodeStatusList'
import type { NodeEntry } from './NodeStatusList'
import type { ActiveRunStatus } from '../../stores/useRunStore'

const statusText: Record<ActiveRunStatus, string> = {
  idle: '',
  running: 'Running…',
  succeeded: 'Run succeeded',
  failed: 'Run failed',
}

const statusColor: Record<ActiveRunStatus, string> = {
  idle: 'text-gray-400',
  running: 'text-amber-400',
  succeeded: 'text-green-400',
  failed: 'text-red-400',
}

export function RunStatusPanel() {
  const panelOpen = useRunStore(s => s.panelOpen)
  const activeRunId = useRunStore(s => s.activeRunId)
  const runStatus = useRunStore(s => s.runStatus)
  const nodeStatuses = useRunStore(s => s.nodeStatuses)
  const connectionLost = useRunStore(s => s.connectionLost)
  const closePanel = useRunStore(s => s.closePanel)
  const nodes = useWorkflowStore(s => s.nodes)
  const workflowId = useWorkflowStore(s => s.workflowId)

  if (!panelOpen) return null

  const entries: NodeEntry[] = nodes.map(n => ({
    nodeId: n.id,
    label: n.data.label as string,
    status: nodeStatuses[n.id],
  }))

  const headerText = connectionLost
    ? 'Connection lost — check run history for status'
    : statusText[runStatus]

  const headerColor = connectionLost
    ? 'text-orange-400'
    : statusColor[runStatus]

  return (
    <div className="absolute bottom-0 left-0 right-0 z-20 bg-gray-900 border-t border-gray-700 shadow-2xl flex flex-col" style={{ maxHeight: '40%' }}>
      {/* Header */}
      <div className="flex items-center gap-3 px-4 py-2 border-b border-gray-700 flex-shrink-0">
        <span className={`text-sm font-semibold ${headerColor}`}>
          {headerText}
        </span>
        {activeRunId && (
          <span className="text-xs text-gray-500 font-mono truncate flex-1">
            run: {activeRunId}
          </span>
        )}
        {workflowId && activeRunId && (
          <Link
            to={`/runs/${activeRunId}`}
            className="text-xs text-indigo-400 hover:text-indigo-300 flex-shrink-0 transition-colors"
          >
            View details
          </Link>
        )}
        <button
          onClick={closePanel}
          className="text-gray-500 hover:text-gray-300 transition-colors flex-shrink-0"
          aria-label="Close run panel"
        >
          ✕
        </button>
      </div>

      {/* Node list */}
      <div className="flex-1 overflow-y-auto px-4 py-2">
        {entries.length === 0 ? (
          <p className="text-xs text-gray-500 italic">No nodes in workflow</p>
        ) : (
          <NodeStatusList entries={entries} />
        )}
      </div>
    </div>
  )
}
