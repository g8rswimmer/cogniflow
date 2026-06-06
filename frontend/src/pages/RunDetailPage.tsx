import { useEffect, useState, memo } from 'react'
import { useParams, Link, useLocation } from 'react-router-dom'
import {
  ReactFlow,
  ReactFlowProvider,
  Background,
  Handle,
  Position,
  BackgroundVariant,
} from '@xyflow/react'
import type { Node, Edge, NodeProps, NodeTypes } from '@xyflow/react'
import '@xyflow/react/dist/style.css'

import { api } from '../hooks/useApi'
import { CanvasControls } from '../components/canvas/CanvasControls'
import type { Run, Workflow } from '../api/types'
import { StatusBadge } from '../components/shared/StatusBadge'
import { NodeStatusList } from '../components/run/NodeStatusList'
import type { NodeEntry } from '../components/run/NodeStatusList'
import type { NodeRunStatus } from '../stores/useRunStore'

// ---------------------------------------------------------------------------
// Static graph node with built-in status colouring (no store dependency)
// ---------------------------------------------------------------------------

interface DetailNodeData {
  type_id: string
  label: string
  runStatus?: 'succeeded' | 'unknown'
  [key: string]: unknown
}

const detailNodeBg: Record<string, string> = {
  succeeded: 'bg-green-800 border-green-500',
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
// Page
// ---------------------------------------------------------------------------

export function RunDetailPage() {
  const { run_id } = useParams<{ run_id: string }>()
  // RunHistoryPage passes workflow_id via location state so both fetches can
  // fire in parallel. Falls back to serial (getRun first) on direct navigation.
  const location = useLocation()
  const stateWorkflowId = (location.state as { workflow_id?: string } | null)?.workflow_id

  const [run, setRun] = useState<Run | null>(null)
  const [workflow, setWorkflow] = useState<Workflow | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!run_id) return

    let alive = true
    setLoading(true)
    setError(null)
    setRun(null)
    setWorkflow(null)

    const load = stateWorkflowId
      ? Promise.all([api.getRun(run_id), api.getWorkflow(stateWorkflowId)])
          .then(([r, wf]) => { if (alive) { setRun(r); setWorkflow(wf) } })
      : api.getRun(run_id)
          .then(async r => {
            if (!alive) return
            setRun(r)
            const wf = await api.getWorkflow(r.workflow_id)
            if (!alive) return
            setWorkflow(wf)
          })

    load
      .catch(err => { if (alive) setError(err instanceof Error ? err.message : 'Failed to load') })
      .finally(() => { if (alive) setLoading(false) })

    return () => { alive = false }
  }, [run_id, stateWorkflowId])

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

  // final_output is null for failed runs (the backend stores the error in
  // error_detail, not final_output). For succeeded runs it contains only
  // sink-node outputs. We use presence in final_output as evidence that a
  // node succeeded, but absence is not evidence of failure — the node may
  // have succeeded without being a sink, or the run may have failed upstream.
  const finalOutput = run.final_output ?? {}

  const rfNodes: Node<DetailNodeData>[] = workflow.nodes.map(n => ({
    id: n.id,
    type: 'detailNode',
    position: n.position,
    data: {
      type_id: n.type_id,
      label: n.label,
      runStatus: finalOutput[n.id] ? 'succeeded' : 'unknown',
    },
  }))

  const rfEdges: Edge[] = workflow.edges.map(e => ({
    id: e.id,
    source: e.source_id,
    target: e.target_id,
    label: e.branch_label ?? undefined,
    style: { stroke: '#6366f1', strokeWidth: 2 },
  }))

  // Build NodeStatusList entries. Only mark nodes as succeeded when there is
  // concrete evidence (output in final_output). Leave everything else without
  // a status so NodeStatusList renders them as neutral gray — we cannot
  // distinguish "failed" from "never ran" from the REST response alone.
  const entries: NodeEntry[] = workflow.nodes.map(n => {
    const output = finalOutput[n.id]
    const status: NodeRunStatus | undefined = output
      ? { status: 'succeeded', output }
      : undefined

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
        <div className="flex items-center gap-3 flex-wrap">
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
            <CanvasControls showInteractive={false} />
          </ReactFlow>
        </ReactFlowProvider>
      </div>

      {/* Node detail list */}
      <div className="flex-1 overflow-y-auto px-4 py-4">
        <h2 className="text-sm font-semibold text-gray-400 uppercase tracking-wider mb-3">
          Node Results
        </h2>
        {run.status === 'failed' && Object.keys(finalOutput).length === 0 && (
          <p className="text-xs text-orange-400 mb-3 italic">
            Run failed before producing output — check error details in the backend logs.
          </p>
        )}
        <NodeStatusList entries={entries} />
      </div>
    </div>
  )
}
