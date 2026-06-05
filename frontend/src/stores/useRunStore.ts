import { create } from 'zustand'
import type { NodeEventType } from '../api/types'

export interface NodeRunStatus {
  status: NodeEventType
  output?: Record<string, unknown>
  error?: string
}

export type ActiveRunStatus = 'idle' | 'running' | 'succeeded' | 'failed'

interface RunStore {
  activeRunId: string | null
  runStatus: ActiveRunStatus
  nodeStatuses: Record<string, NodeRunStatus>
  panelOpen: boolean

  startRun: (runId: string) => void
  setNodeStatus: (
    nodeId: string,
    type: NodeEventType,
    output?: Record<string, unknown>,
    error?: string,
  ) => void
  setRunFinished: (status: 'succeeded' | 'failed') => void
  closePanel: () => void
  clearRun: () => void
}

export const useRunStore = create<RunStore>((set) => ({
  activeRunId: null,
  runStatus: 'idle',
  nodeStatuses: {},
  panelOpen: false,

  startRun: (runId) =>
    set({ activeRunId: runId, runStatus: 'running', nodeStatuses: {}, panelOpen: true }),

  setNodeStatus: (nodeId, type, output, error) =>
    set(s => ({
      nodeStatuses: {
        ...s.nodeStatuses,
        [nodeId]: { status: type, output, error },
      },
    })),

  setRunFinished: (status) =>
    set({ runStatus: status }),

  closePanel: () => set({ panelOpen: false }),

  clearRun: () =>
    set({ activeRunId: null, runStatus: 'idle', nodeStatuses: {}, panelOpen: false }),
}))
