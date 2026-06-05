import { create } from 'zustand'
import type { NodeMeta } from '../api/types'
import { api } from '../hooks/useApi'

interface NodeTypeStore {
  nodeTypes: NodeMeta[]
  loaded: boolean
  loading: boolean
  error: string | null
  load: () => Promise<void>
  byTypeId: (typeId: string) => NodeMeta | undefined
}

export const useNodeTypeStore = create<NodeTypeStore>((set, get) => ({
  nodeTypes: [],
  loaded: false,
  loading: false,
  error: null,

  load: async () => {
    // Guard against both a completed load and an in-flight request so that
    // concurrent callers (e.g. NodePalette + WorkflowEditorPage mounting
    // simultaneously) do not issue duplicate GET /node-types requests.
    const { loaded, loading } = get()
    if (loaded || loading) return
    set({ loading: true })
    try {
      const { node_types } = await api.listNodeTypes()
      set({ nodeTypes: node_types, loaded: true, loading: false, error: null })
    } catch (err) {
      set({ loading: false, error: err instanceof Error ? err.message : 'Failed to load node types' })
    }
  },

  byTypeId: (typeId) => get().nodeTypes.find(n => n.type_id === typeId),
}))
