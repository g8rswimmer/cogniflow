import { useState } from 'react'
import type { NodeMock } from '../../api/types'

const selectCls =
  'w-full rounded-md bg-gray-900 border border-gray-600 px-3 py-1.5 text-sm text-gray-100 focus:outline-none focus:border-indigo-500'

const textareaCls =
  'w-full rounded-md bg-gray-900 border border-gray-600 px-3 py-1.5 text-sm text-gray-100 font-mono placeholder-gray-500 focus:outline-none focus:border-indigo-500 resize-y'

export interface NodeOption {
  id: string
  label: string
}

interface Props {
  mock: NodeMock
  nodes: NodeOption[]
  onChange: (mock: NodeMock) => void
  onRemove: () => void
  nodeError?: string
  outputError?: string
}

export function MockEditor({ mock, nodes, onChange, onRemove, nodeError, outputError }: Props) {
  const safeNodes = (nodes ?? []).filter(
    (n): n is NodeOption => n != null && typeof n === 'object' && typeof n.id === 'string',
  )
  const [outputText, setOutputText] = useState(() =>
    Object.keys(mock.output ?? {}).length ? JSON.stringify(mock.output, null, 2) : ''
  )
  const [jsonError, setJsonError] = useState<string | null>(null)

  const handleOutputChange = (text: string) => {
    setOutputText(text)
    if (text.trim() === '') {
      setJsonError(null)
      onChange({ ...mock, output: {} })
      return
    }
    try {
      const parsed = JSON.parse(text)
      if (typeof parsed !== 'object' || Array.isArray(parsed) || parsed === null) {
        setJsonError('Must be a JSON object { … }')
        return
      }
      setJsonError(null)
      onChange({ ...mock, output: parsed as Record<string, unknown> })
    } catch {
      setJsonError('Invalid JSON')
    }
  }

  return (
    <div className="border border-gray-700 rounded-lg p-3 space-y-3" style={{ background: '#1a2236' }}>
      <div className="flex items-start justify-between gap-2">
        <div className="flex-1 space-y-1">
          <label className="block text-xs font-semibold text-gray-300">Node to mock</label>
          {nodeError && <p className="text-xs text-red-400">{nodeError}</p>}
          <select
            className={selectCls}
            value={mock.node_id}
            onChange={e => onChange({ ...mock, node_id: e.target.value })}
          >
            <option value="">— select a node —</option>
            {safeNodes.map(n => (
              <option key={n.id} value={n.id}>{n.label || n.id} ({n.id})</option>
            ))}
          </select>
        </div>
        <button
          type="button"
          onClick={onRemove}
          className="mt-5 text-gray-500 hover:text-red-400 transition-colors text-xs px-1 flex-shrink-0"
          title="Remove mock"
        >
          ✕
        </button>
      </div>

      <div className="space-y-1">
        <label className="block text-xs font-semibold text-gray-300">Mock output (JSON object)</label>
        {outputError && <p className="text-xs text-red-400">{outputError}</p>}
        <textarea
          rows={4}
          className={textareaCls}
          placeholder={'{\n  "status_code": 200,\n  "body": "ok"\n}'}
          value={outputText}
          onChange={e => handleOutputChange(e.target.value)}
        />
        {jsonError && <p className="text-xs text-red-400">{jsonError}</p>}
      </div>
    </div>
  )
}
