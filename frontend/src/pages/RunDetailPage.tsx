import { useEffect, useState, memo } from 'react'
import { useParams, Link } from 'react-router-dom'
import {
  ReactFlow,
  ReactFlowProvider,
  Background,
  Controls,
  Handle,
  Position,
  BackgroundVariant,
} from '@xyflow/react'
import type { Node, Edge, NodeProps, NodeTypes } from '@xyflow/react'
import '@xyflow/react/dist/style.css'

import { api } from '../hooks/useApi'
import type { Run, RunStatus, Workflow } from '../api/types'
import { NodeStatusList } from '../components/run/NodeStatusList'
import type { NodeEntry } from '../components/run/NodeStatusList'
import type { NodeRunStatus } from '../stores/useRunStore'

// ---------------------------------------------------------------------------
// Static graph node with built-in status colouring (no store dependency)
// ---------------------------------------------------------------------------

interface DetailNodeData {
  type_id: string
  label: string
  runStatus?: 'succeeded' | 'failed' | 'unknown'
  [key: string]: unknown
}

const detailNodeBg: Record<string, string> = {
  succeeded: 'bg-green-800 border-green-500',
  failed: 'bg-red-800 border-red-500',
  unknown: 'bg-gray-700 border-gray-500',
}

function DetailNode({ data }: NodeProps<Node<DetailNodeData>>) {
  const colorClass = detailNodeBg[data.runStatus ?? 'unknown'] ?? detailNodeBg.unknown
  return (
    <div className={`min-w-[160px] rounded-lg border-2 shadow-lg ${colorClass}`}>
      <Handle type="target" position={Position.Top} className="!bg-gray-300 !border-gray-500" />
      <div className="px-3 py-2">
        <div className="text-xs text-gray-300 font-mono truncate">{data.type_id}</div>
        <div className="text-sm text-white font-semibold mt-0.5 truncate">{data.label}</div>
      </div>
      <Handle type="source" position={Position.Bottom} className="!bg-gray-300 !border-gray-500" />
    </div>
  )
}
const MemoDetailNode = memo(DetailNode)

const detailNodeTypes: NodeTypes = { detailNode: MemoDetailNode }

// ---------------------------------------------------------------------------
// Status badge
// ---------------------------------------------------------------------------

const statusColors: Record<RunStatus, string> = {
  pending: 'bg-gray-600 text-gray-300',
  running: 'bg-amber-700 text-amber-200',
  succeeded: 'bg-green-700 text-green-200',
  failed: 'bg-red-700 text-red-200',
}

function StatusBadge({ status }: { status: RunStatus }) {
  return (
    <span className={`px-2 py-0.5 rounded text-xs font-semibold ${statusColors[status] ?? 'bg-gray-600 text-gray-300'}`}>
      {status}
    </span>
  )
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export function RunDetailPage() {
  const { run_id } = useParams<{ run_id: string }>()
  const [run, setRun] = useState<Run | null>(null)
  const [workflow, setWorkflow] = useState<Workflow | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!run_id) return

    setLoading(true)
    api.getRun(run_id)
      .then(async r => {
        setRun(r)
        // Fetch the workflow to get node positions and labels for the graph.
        const wf = await api.getWorkflow(r.workflow_id)
        setWorkflow(wf)
      })
      .catch(err => setError(err instanceof Error ? err.message : 'Failed to load'))
      .finally(() => setLoading(false))
  }, [run_id])

  if (loading) {
    return (
      <div className="min-h-screen bg-gray-950 flex items-center justify-center">
        <p className="text-gray-400">Loading…</p>
      </div>
    )
  }

  if (error || !run || !workflow) {
    return (
      <div className="min-h-screen bg-gray-950 flex items-center justify-center">
        <div className="text-center">
          <p className="text-red-400 mb-4">{error ?? 'Run not found'}</p>
          <Link to="/" className="text-sm text-indigo-400 hover:text-indigo-300">← Workflows</Link>
        </div>
      </div>
    )
  }

  const finalOutput = run.final_output ?? {}

  // Build React Flow nodes with status colouring derived from final_output.
  const rfNodes: Node<DetailNodeData>[] = workflow.nodes.map(n => {
    const hasOutput = !!finalOutput[n.id]
    const runStatus =
      hasOutput ? 'succeeded'
        : run.status === 'failed' ? 'failed'
        : 'unknown'

    return {
      id: n.id,
      type: 'detailNode',
      position: n.position,
      data: { type_id: n.type_id, label: n.label, runStatus },
    }
  })

  const rfEdges: Edge[] = workflow.edges.map(e => ({
    id: e.id,
    source: e.source_id,
    target: e.target_id,
    label: e.branch_label ?? undefined,
    style: { stroke: '#6366f1', strokeWidth: 2 },
  }))

  // Build NodeStatusList entries from final_output.
  const entries: NodeEntry[] = workflow.nodes.map(n => {
    const output = finalOutput[n.id]
    let status: NodeRunStatus | undefined

    if (output) {
      status = { status: 'node.succeeded', output }
    } else if (run.status === 'failed') {
      status = { status: 'node.failed', error: 'Node did not produce output' }
    }

    return { nodeId: n.id, label: n.label, status }
  })

  const durationMs = run.finished_at
    ? new Date(run.finished_at).getTime() - new Date(run.started_at).getTime()
    : null

  return (
    <div className="min-h-screen bg-gray-950 text-gray-100 flex flex-col">
      {/* Top bar */}
      <div className="flex items-center gap-4 px-4 py-3 border-b border-gray-800 flex-shrink-0">
        <Link
          to={`/workflows/${run.workflow_id}/runs`}
          className="text-indigo-400 hover:text-indigo-300 text-sm transition-colors"
        >
          ← Run History
        </Link>
        <div className="flex items-center gap-3">
          <StatusBadge status={run.status} />
          <span className="text-sm text-gray-400 font-mono truncate max-w-xs">{run.run_id}</span>
          <span className="text-xs text-gray-500">
            {new Date(run.started_at).toLocaleString()}
            {durationMs !== null && ` · ${(durationMs / 1000).toFixed(1)}s`}
          </span>
          <span className="text-xs text-gray-600">triggered by {run.triggered_by}</span>
        </div>
      </div>

      {/* Graph snapshot */}
      <div className="flex-shrink-0" style={{ height: '50vh' }}>
        <ReactFlowProvider>
          <ReactFlow
            nodes={rfNodes}
            edges={rfEdges}
            nodeTypes={detailNodeTypes}
            fitView
            nodesDraggable={false}
            nodesConnectable={false}
            elementsSelectable={false}
            panOnScroll
            className="bg-gray-950"
            defaultEdgeOptions={{ animated: false }}
          >
            <Background variant={BackgroundVariant.Dots} color="#374151" gap={20} />
            <Controls className="!bg-gray-800 !border-gray-700 !text-gray-100" />
          </ReactFlow>
        </ReactFlowProvider>
      </div>

      {/* Node detail list */}
      <div className="flex-1 overflow-y-auto px-4 py-4">
        <h2 className="text-sm font-semibold text-gray-400 uppercase tracking-wider mb-3">
          Node Results
        </h2>
        <NodeStatusList entries={entries} />
      </div>
    </div>
  )
}
