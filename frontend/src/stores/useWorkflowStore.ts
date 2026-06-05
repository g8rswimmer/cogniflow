import { create } from 'zustand'
import {
  applyNodeChanges,
  applyEdgeChanges,
  addEdge,
} from '@xyflow/react'
import type {
  Node,
  Edge,
  NodeChange,
  EdgeChange,
  Connection,
} from '@xyflow/react'
import type { Trigger, OutputParser, Workflow } from '../api/types'

export interface NodeData {
  type_id: string
  label: string
  [key: string]: unknown
}

export type WorkflowNode = Node<NodeData>

const defaultTrigger: Trigger = { kind: 'manual' }

interface WorkflowStore {
  // Persisted identity
  workflowId: string | null
  name: string
  trigger: Trigger
  timeoutSeconds: number
  isDirty: boolean

  // Canvas
  nodes: WorkflowNode[]
  edges: Edge[]
  selectedNodeId: string | null

  // Per-node config & parsers
  configs: Record<string, Record<string, unknown>>
  outputParsers: Record<string, Record<string, OutputParser>>

  // React Flow handlers
  onNodesChange: (changes: NodeChange<WorkflowNode>[]) => void
  onEdgesChange: (changes: EdgeChange[]) => void
  onConnect: (connection: Connection) => void

  // Actions
  setName: (name: string) => void
  setTrigger: (trigger: Trigger) => void
  setTimeoutSeconds: (s: number) => void
  addNode: (node: WorkflowNode) => void
  selectNode: (id: string | null) => void
  updateNodeLabel: (nodeId: string, label: string) => void
  updateNodeConfig: (nodeId: string, config: Record<string, unknown>) => void
  updateOutputParsers: (nodeId: string, parsers: Record<string, OutputParser>) => void
  updateEdgeLabel: (edgeId: string, label: string | null) => void

  // Load / reset
  loadWorkflow: (wf: Workflow) => void
  reset: () => void
  markClean: (id: string) => void
}

export const useWorkflowStore = create<WorkflowStore>((set) => ({
  workflowId: null,
  name: 'Untitled Workflow',
  trigger: defaultTrigger,
  timeoutSeconds: 60,
  isDirty: false,

  nodes: [],
  edges: [],
  selectedNodeId: null,

  configs: {},
  outputParsers: {},

  onNodesChange: (changes) =>
    set(s => {
      const nextNodes = applyNodeChanges(changes, s.nodes)

      // Only mark dirty for user-initiated structural changes. React Flow fires
      // 'select' and 'dimensions' changes internally (e.g. on fitView load) which
      // should not flip the unsaved-changes flag.
      const userChange = changes.some(
        c => c.type === 'add' || c.type === 'remove' || c.type === 'position',
      )

      // Prune configs and outputParsers for any removed nodes so stale data
      // does not accumulate in the store across the session.
      const removedIds = changes
        .filter(c => c.type === 'remove')
        .map(c => c.id)

      if (removedIds.length > 0) {
        const configs = { ...s.configs }
        const outputParsers = { ...s.outputParsers }
        for (const id of removedIds) {
          delete configs[id]
          delete outputParsers[id]
        }
        return { nodes: nextNodes, configs, outputParsers, isDirty: true }
      }

      return { nodes: nextNodes, isDirty: userChange ? true : s.isDirty }
    }),

  onEdgesChange: (changes) =>
    set(s => ({
      edges: applyEdgeChanges(changes, s.edges),
      isDirty: true,
    })),

  onConnect: (connection) =>
    set(s => ({
      edges: addEdge({ ...connection, type: 'labeled' }, s.edges),
      isDirty: true,
    })),

  setName: (name) => set({ name, isDirty: true }),
  setTrigger: (trigger) => set({ trigger, isDirty: true }),
  setTimeoutSeconds: (timeoutSeconds) => set({ timeoutSeconds, isDirty: true }),

  addNode: (node) =>
    set(s => ({ nodes: [...s.nodes, node], isDirty: true })),

  selectNode: (selectedNodeId) =>
    set(s => ({
      selectedNodeId,
      // Pre-initialize config so `configs[id] ?? {}` never creates a new object
      // reference on every render, which would cause RJSF's onChange infinite loop.
      configs: selectedNodeId && !s.configs[selectedNodeId]
        ? { ...s.configs, [selectedNodeId]: {} }
        : s.configs,
    })),

  updateNodeLabel: (nodeId, label) =>
    set(s => ({
      nodes: s.nodes.map(n =>
        n.id === nodeId ? { ...n, data: { ...n.data, label } } : n,
      ),
      isDirty: true,
    })),

  updateNodeConfig: (nodeId, config) =>
    set(s => ({
      configs: { ...s.configs, [nodeId]: config },
      isDirty: true,
    })),

  updateOutputParsers: (nodeId, parsers) =>
    set(s => ({
      outputParsers: { ...s.outputParsers, [nodeId]: parsers },
      isDirty: true,
    })),

  updateEdgeLabel: (edgeId, label) =>
    set(s => ({
      edges: s.edges.map(e =>
        e.id === edgeId ? { ...e, label: label ?? undefined } : e,
      ),
      isDirty: true,
    })),

  loadWorkflow: (wf) => {
    const nodes: WorkflowNode[] = wf.nodes.map(n => ({
      id: n.id,
      type: 'workflowNode',
      position: n.position,
      data: { type_id: n.type_id, label: n.label },
    }))

    const edges: Edge[] = wf.edges.map(e => ({
      id: e.id,
      type: 'labeled',
      source: e.source_id,
      target: e.target_id,
      label: e.branch_label ?? undefined,
    }))

    const configs: Record<string, Record<string, unknown>> = {}
    const outputParsers: Record<string, Record<string, OutputParser>> = {}
    for (const n of wf.nodes) {
      configs[n.id] = n.config ?? {}
      if (n.output_parsers) outputParsers[n.id] = n.output_parsers
    }

    set({
      workflowId: wf.id,
      name: wf.name,
      trigger: wf.trigger,
      timeoutSeconds: wf.timeout_seconds,
      nodes,
      edges,
      configs,
      outputParsers,
      selectedNodeId: null,
      isDirty: false,
    })
  },

  reset: () =>
    set({
      workflowId: null,
      name: 'Untitled Workflow',
      trigger: defaultTrigger,
      timeoutSeconds: 60,
      nodes: [],
      edges: [],
      configs: {},
      outputParsers: {},
      selectedNodeId: null,
      isDirty: false,
    }),

  markClean: (id) => set({ workflowId: id, isDirty: false }),
}))

// Utility: find all ancestor node IDs for a given node
export function getAncestors(nodeId: string, edges: Edge[]): string[] {
  const ancestors = new Set<string>()
  const queue = [nodeId]
  while (queue.length > 0) {
    const current = queue.shift()!
    for (const edge of edges) {
      if (edge.target === current && !ancestors.has(edge.source)) {
        ancestors.add(edge.source)
        queue.push(edge.source)
      }
    }
  }
  return [...ancestors]
}
