import type { NodeMeta } from '../../api/types'

interface Props {
  node: NodeMeta
}

export function PaletteNodeCard({ node }: Props) {
  const onDragStart = (e: React.DragEvent) => {
    e.dataTransfer.setData('application/cogniflow-type-id', node.type_id)
    e.dataTransfer.setData('application/cogniflow-display-name', node.display_name)
    e.dataTransfer.effectAllowed = 'move'
  }

  return (
    <div
      draggable
      onDragStart={onDragStart}
      className="
        cursor-grab active:cursor-grabbing
        rounded-md border border-gray-600 bg-gray-700
        px-3 py-2 hover:bg-gray-600 hover:border-gray-500
        transition-colors select-none
      "
    >
      <div className="text-sm font-medium text-gray-100 truncate">{node.display_name}</div>
      {node.description && (
        <div className="text-xs text-gray-400 mt-0.5 line-clamp-2">{node.description}</div>
      )}
    </div>
  )
}
