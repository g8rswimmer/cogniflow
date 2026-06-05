import { create } from 'zustand'
import type { NodeMeta } from '../api/types'
import { api } from '../hooks/useApi'

interface NodeTypeStore {
  nodeTypes: NodeMeta[]
  loaded: boolean
  error: string | null
  load: () => Promise<void>
  byTypeId: (typeId: string) => NodeMeta | undefined
}

export const useNodeTypeStore = create<NodeTypeStore>((set, get) => ({
  nodeTypes: [],
  loaded: false,
  error: null,

  load: async () => {
    if (get().loaded) return
    try {
      const { node_types } = await api.listNodeTypes()
      set({ nodeTypes: node_types, loaded: true, error: null })
    } catch (err) {
      set({ error: err instanceof Error ? err.message : 'Failed to load node types' })
    }
  },

  byTypeId: (typeId) => get().nodeTypes.find(n => n.type_id === typeId),
}))
