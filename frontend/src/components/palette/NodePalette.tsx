import { useState, useMemo } from 'react'
import { useNodeTypeStore } from '../../stores/useNodeTypeStore'
import { PaletteNodeCard } from './PaletteNodeCard'

export function NodePalette() {
  const nodeTypes = useNodeTypeStore(s => s.nodeTypes)
  const [query, setQuery] = useState('')

  const filtered = useMemo(() => {
    const q = query.toLowerCase()
    return q
      ? nodeTypes.filter(
          n =>
            n.display_name.toLowerCase().includes(q) ||
            n.type_id.toLowerCase().includes(q) ||
            n.category.toLowerCase().includes(q),
        )
      : nodeTypes
  }, [nodeTypes, query])

  const byCategory = useMemo(() => {
    const map: Record<string, typeof filtered> = {}
    for (const n of filtered) {
      if (!map[n.category]) map[n.category] = []
      map[n.category].push(n)
    }
    return map
  }, [filtered])

  return (
    <aside className="w-56 flex-shrink-0 border-r border-gray-700 bg-gray-800 flex flex-col overflow-hidden">
      <div className="p-2 border-b border-gray-700">
        <input
          type="search"
          value={query}
          onChange={e => setQuery(e.target.value)}
          placeholder="Search nodes…"
          className="
            w-full rounded-md bg-gray-700 border border-gray-600
            px-2 py-1.5 text-sm text-gray-100 placeholder-gray-400
            focus:outline-none focus:border-indigo-500
          "
        />
      </div>

      <div className="flex-1 overflow-y-auto p-2 space-y-4">
        {Object.entries(byCategory).map(([category, nodes]) => (
          <div key={category}>
            <div className="text-xs font-semibold uppercase tracking-wider text-gray-400 mb-1 px-1">
              {category}
            </div>
            <div className="space-y-1">
              {nodes.map(n => (
                <PaletteNodeCard key={n.type_id} node={n} />
              ))}
            </div>
          </div>
        ))}

        {filtered.length === 0 && (
          <p className="text-sm text-gray-500 text-center pt-4">No nodes found</p>
        )}
      </div>
    </aside>
  )
}
