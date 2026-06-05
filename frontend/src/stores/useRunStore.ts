import { create } from 'zustand'
import type { NodeEventType } from '../api/types'
import type { NodeState } from '../lib/nodeStatus'

export interface NodeRunStatus {
  status: NodeState
  output?: Record<string, unknown>
  error?: string
}

export type ActiveRunStatus = 'idle' | 'running' | 'succeeded' | 'failed'

// Map node-level event types to the node-state enum. run.* events are not
// valid per-node states and are intentionally excluded so that an accidental
// call with a run-level type is a no-op rather than corrupting the store.
const nodeEventToState: Partial<Record<NodeEventType, NodeState>> = {
  'node.pending': 'pending',
  'node.running': 'running',
  'node.succeeded': 'succeeded',
  'node.failed': 'failed',
}

interface RunStore {
  activeRunId: string | null
  runStatus: ActiveRunStatus
  nodeStatuses: Record<string, NodeRunStatus>
  panelOpen: boolean
  connectionLost: boolean

  startRun: (runId: string) => void
  setNodeStatus: (
    nodeId: string,
    type: NodeEventType,
    output?: Record<string, unknown>,
    error?: string,
  ) => void
  setRunFinished: (status: 'succeeded' | 'failed') => void
  setConnectionLost: () => void
  closePanel: () => void
  clearRun: () => void
}

export const useRunStore = create<RunStore>((set) => ({
  activeRunId: null,
  runStatus: 'idle',
  nodeStatuses: {},
  panelOpen: false,
  connectionLost: false,

  startRun: (runId) =>
    set({
      activeRunId: runId,
      runStatus: 'running',
      nodeStatuses: {},
      panelOpen: true,
      connectionLost: false,
    }),

  setNodeStatus: (nodeId, type, output, error) => {
    const state = nodeEventToState[type]
    if (!state) return
    set(s => ({
      nodeStatuses: {
        ...s.nodeStatuses,
        [nodeId]: { status: state, output, error },
      },
    }))
  },

  setRunFinished: (status) =>
    set({ runStatus: status }),

  // Transport error — the run may still be executing on the backend.
  // Do not mark runStatus as 'failed'; instead surface a connection-lost
  // indicator so the user knows to check run history for the true outcome.
  setConnectionLost: () => set({ connectionLost: true }),

  closePanel: () => set({ panelOpen: false }),

  clearRun: () =>
    set({
      activeRunId: null,
      runStatus: 'idle',
      nodeStatuses: {},
      panelOpen: false,
      connectionLost: false,
    }),
}))
