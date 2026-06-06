import { useCallback, useState, useRef } from 'react'
import { getBezierPath, EdgeLabelRenderer, BaseEdge } from '@xyflow/react'
import type { EdgeProps } from '@xyflow/react'
import { useWorkflowStore } from '../../stores/useWorkflowStore'

export function LabeledEdge({
  id,
  sourceX,
  sourceY,
  sourcePosition,
  targetX,
  targetY,
  targetPosition,
  label,
}: EdgeProps) {
  const updateEdgeLabel = useWorkflowStore(s => s.updateEdgeLabel)
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState('')
  const inputRef = useRef<HTMLInputElement>(null)

  const [edgePath, labelX, labelY] = getBezierPath({
    sourceX,
    sourceY,
    sourcePosition,
    targetX,
    targetY,
    targetPosition,
  })

  const startEdit = useCallback(() => {
    setDraft(typeof label === 'string' ? label : '')
    setEditing(true)
    requestAnimationFrame(() => {
      inputRef.current?.focus()
      inputRef.current?.select()
    })
  }, [label])

  const commit = useCallback(() => {
    setEditing(false)
    updateEdgeLabel(id, draft.trim() || null)
  }, [id, draft, updateEdgeLabel])

  return (
    <>
      <BaseEdge
        path={edgePath}
        style={{ stroke: '#6366f1', strokeWidth: 2 }}
      />
      <EdgeLabelRenderer>
        <div
          style={{
            position: 'absolute',
            transform: `translate(-50%, -50%) translate(${labelX}px,${labelY}px)`,
            pointerEvents: 'all',
          }}
          className="nodrag nopan"
        >
          {editing ? (
            <input
              ref={inputRef}
              value={draft}
              onChange={e => setDraft(e.target.value)}
              onBlur={commit}
              onKeyDown={e => {
                if (e.key === 'Enter') commit()
                if (e.key === 'Escape') setEditing(false)
              }}
              className="w-24 text-xs text-center bg-gray-800 border border-indigo-500 rounded px-1.5 py-0.5 text-gray-100 outline-none"
              placeholder="rule label"
            />
          ) : (
            <button
              onClick={startEdit}
              title="Click to set branch label"
              className={[
                'text-xs px-2 py-0.5 rounded transition-colors',
                label
                  ? 'bg-indigo-900 text-indigo-200 border border-indigo-600 hover:bg-indigo-800'
                  : 'bg-transparent text-gray-600 border border-dashed border-gray-700 hover:border-gray-500 hover:text-gray-400',
              ].join(' ')}
            >
              {label || '+label'}
            </button>
          )}
        </div>
      </EdgeLabelRenderer>
    </>
  )
}
