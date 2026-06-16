import { useEffect, useCallback, useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import {
  ReactFlow,
  ReactFlowProvider,
  Background,
  MiniMap,
  useReactFlow,
  BackgroundVariant,
} from '@xyflow/react'
import type { NodeTypes, EdgeTypes } from '@xyflow/react'
import '@xyflow/react/dist/style.css'

import { useWorkflowStore } from '../stores/useWorkflowStore'
import { useNodeTypeStore } from '../stores/useNodeTypeStore'
import { useRunStore } from '../stores/useRunStore'
import { useToastStore } from '../stores/useToastStore'
import { useRunEvents } from '../hooks/useRunEvents'
import { api } from '../hooks/useApi'
import { ApiError } from '../api/client'
import { Navbar } from '../components/shared/Navbar'
import { NodePalette } from '../components/palette/NodePalette'
import { ConfigSidebar } from '../components/sidebar/ConfigSidebar'
import { RunStatusPanel } from '../components/run/RunStatusPanel'
import { InitialDataModal } from '../components/run/InitialDataModal'
import CustomNode from '../components/canvas/CustomNode'
import { LabeledEdge } from '../components/canvas/LabeledEdge'
import { CanvasControls } from '../components/canvas/CanvasControls'

// Defined outside the component to prevent React Flow re-render warnings.
const nodeTypes: NodeTypes = { workflowNode: CustomNode }
const edgeTypes: EdgeTypes = { labeled: LabeledEdge }

// Inner canvas component — must live inside ReactFlowProvider to call useReactFlow().
function EditorCanvas() {
  const { screenToFlowPosition } = useReactFlow()

  const nodes = useWorkflowStore(s => s.nodes)
  const edges = useWorkflowStore(s => s.edges)
  const onNodesChange = useWorkflowStore(s => s.onNodesChange)
  const onEdgesChange = useWorkflowStore(s => s.onEdgesChange)
  const onConnect = useWorkflowStore(s => s.onConnect)
  const addNode = useWorkflowStore(s => s.addNode)
  const selectNode = useWorkflowStore(s => s.selectNode)
  const byTypeId = useNodeTypeStore(s => s.byTypeId)

  const onDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    e.dataTransfer.dropEffect = 'move'
  }, [])

  const onDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault()
      const typeId = e.dataTransfer.getData('application/cogniflow-type-id')
      const displayName = e.dataTransfer.getData('application/cogniflow-display-name')
      if (!typeId) return

      const position = screenToFlowPosition({ x: e.clientX, y: e.clientY })
      // Replace dots and hyphens so the ID is a valid Go template identifier.
      // e.g. "llm.anthropic" → "llm_anthropic_1780706153289"
      const id = `${typeId.replace(/[^a-zA-Z0-9]/g, '_')}_${Date.now()}`
      const meta = byTypeId(typeId)

      addNode({
        id,
        type: 'workflowNode',
        position,
        data: { type_id: typeId, label: meta?.display_name ?? displayName ?? typeId },
      })
    },
    [screenToFlowPosition, addNode, byTypeId],
  )

  const onNodeClick = useCallback(
    (_: React.MouseEvent, node: { id: string }) => {
      selectNode(node.id)
    },
    [selectNode],
  )

  const onPaneClick = useCallback(() => {
    selectNode(null)
  }, [selectNode])

  return (
    <div className="flex-1 relative">
      <ReactFlow
        nodes={nodes}
        edges={edges}
        nodeTypes={nodeTypes}
        edgeTypes={edgeTypes}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        onConnect={onConnect}
        onNodeClick={onNodeClick}
        onPaneClick={onPaneClick}
        onDrop={onDrop}
        onDragOver={onDragOver}
        fitView
        deleteKeyCode="Delete"
        className="bg-gray-950"
        defaultEdgeOptions={{ animated: false, type: 'labeled' }}
      >
        <Background variant={BackgroundVariant.Dots} color="#374151" gap={20} />
        <CanvasControls />
        <MiniMap
          className="!bg-gray-800 !border-gray-700"
          nodeColor="#6366f1"
          maskColor="rgba(0,0,0,0.5)"
        />
      </ReactFlow>

      <RunStatusPanel />
    </div>
  )
}

export function WorkflowEditorPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const isNew = !id || id === 'new'

  const reset = useWorkflowStore(s => s.reset)
  const loadWorkflow = useWorkflowStore(s => s.loadWorkflow)
  const workflowId = useWorkflowStore(s => s.workflowId)
  const name = useWorkflowStore(s => s.name)
  const trigger = useWorkflowStore(s => s.trigger)
  const timeoutSeconds = useWorkflowStore(s => s.timeoutSeconds)
  const nodes = useWorkflowStore(s => s.nodes)
  const edges = useWorkflowStore(s => s.edges)
  const configs = useWorkflowStore(s => s.configs)
  const outputParsers = useWorkflowStore(s => s.outputParsers)
  const initialDataSchema = useWorkflowStore(s => s.initialDataSchema)
  const markClean = useWorkflowStore(s => s.markClean)

  const loadNodeTypes = useNodeTypeStore(s => s.load)

  const setValidationErrors = useWorkflowStore(s => s.setValidationErrors)
  const clearValidationErrors = useWorkflowStore(s => s.clearValidationErrors)

  const activeRunId = useRunStore(s => s.activeRunId)
  const runStatus = useRunStore(s => s.runStatus)
  const startRun = useRunStore(s => s.startRun)
  const clearRun = useRunStore(s => s.clearRun)

  const addToast = useToastStore(s => s.addToast)

  const [loadError, setLoadError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [showInitialDataModal, setShowInitialDataModal] = useState(false)

  // Subscribe to WebSocket events for the active run.
  useRunEvents(activeRunId)

  // Load node types once on mount.
  useEffect(() => { loadNodeTypes() }, [loadNodeTypes])

  // Load or reset workflow on route change; clear any stale run state.
  // Zustand actions (clearRun, reset, loadWorkflow) are stable references —
  // including them in deps is safe and keeps the lint rule satisfied.
  useEffect(() => {
    clearRun()
    if (isNew) {
      reset()
    } else if (id) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setLoadError(null)
      api.getWorkflow(id)
        .then(wf => loadWorkflow(wf))
        .catch(err => setLoadError(err instanceof Error ? err.message : 'Failed to load'))
    }
  }, [id, isNew, clearRun, reset, loadWorkflow])

  const buildPayload = () => ({
    name,
    description: useWorkflowStore.getState().description,
    trigger,
    timeout_seconds: timeoutSeconds,
    initial_data_schema: initialDataSchema,
    nodes: nodes.map(n => ({
      id: n.id,
      type_id: n.data.type_id,
      label: n.data.label,
      position: n.position,
      config: configs[n.id] ?? {},
      output_parsers: outputParsers[n.id] ?? {},
    })),
    edges: edges.map(e => ({
      id: e.id,
      source_id: e.source,
      target_id: e.target,
      branch_label: (e.label as string | undefined) ?? null,
    })),
  })

  const handleSave = async () => {
    setSaving(true)
    clearValidationErrors()
    try {
      if (isNew || !workflowId) {
        const wf = await api.createWorkflow(buildPayload())
        markClean(wf.id)
        navigate(`/workflows/${wf.id}`, { replace: true })
      } else {
        await api.updateWorkflow(workflowId, buildPayload())
        markClean(workflowId)
      }
      addToast('success', 'Workflow saved')
    } catch (err) {
      if (err instanceof ApiError) {
        if (err.validationErrors.length > 0) {
          setValidationErrors(err.validationErrors)
          const nodeCount = new Set(
            err.validationErrors.filter(e => e.node_id).map(e => e.node_id)
          ).size
          const hint = nodeCount > 0
            ? ` — ${nodeCount} node${nodeCount > 1 ? 's' : ''} highlighted on canvas`
            : ''
          addToast('error', 'Validation failed', err.message + hint)
          console.error('[save] validation errors:', err.validationErrors)
        } else {
          addToast('error', 'Save failed', err.message)
          console.error('[save] api error:', { code: err.code, message: err.message })
        }
      } else {
        addToast('error', 'Save failed', 'Unexpected error — check console')
        console.error('[save] unexpected error:', err)
      }
    } finally {
      setSaving(false)
    }
  }

  const handleRun = () => {
    if (!workflowId) return
    setShowInitialDataModal(true)
  }

  const handleRunWithData = async (initialData: Record<string, unknown>) => {
    setShowInitialDataModal(false)
    if (!workflowId) return
    try {
      const result = await api.triggerRun(workflowId, initialData)
      startRun(result.run_id)
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Run failed to start'
      addToast('error', 'Run failed', msg)
      console.error('[run]', err)
    }
  }

  if (loadError) {
    return (
      <div className="min-h-screen bg-gray-950 flex items-center justify-center">
        <div className="text-center">
          <p className="text-red-400 mb-4">{loadError}</p>
          <button
            onClick={() => navigate('/')}
            className="text-sm text-indigo-400 hover:text-indigo-300"
          >
            ← Back to workflows
          </button>
        </div>
      </div>
    )
  }

  return (
    <div className="h-screen flex flex-col bg-gray-950 overflow-hidden">
      {showInitialDataModal && (
        <InitialDataModal
          schema={initialDataSchema}
          onRun={handleRunWithData}
          onCancel={() => setShowInitialDataModal(false)}
        />
      )}
      <Navbar
        onSave={handleSave}
        onRun={handleRun}
        saving={saving}
        running={runStatus === 'running'}
      />

      <div className="flex flex-1 overflow-hidden">
        <NodePalette />

        <ReactFlowProvider>
          <EditorCanvas />
        </ReactFlowProvider>

        <ConfigSidebar />
      </div>
    </div>
  )
}
