import { useState } from 'react'
import { useWorkflowStore } from '../../stores/useWorkflowStore'
import type { Trigger, TriggerKind } from '../../api/types'

interface Props {
  workflowId: string | null
  onClose: () => void
}

export function TriggerPanel({ workflowId, onClose }: Props) {
  const trigger = useWorkflowStore(s => s.trigger)
  const setTrigger = useWorkflowStore(s => s.setTrigger)

  const [kind, setKind] = useState<TriggerKind>(trigger.kind)
  const [cronExpr, setCronExpr] = useState(trigger.cron_expr ?? '* * * * *')

  const webhookUrl = workflowId ? `/webhooks/${workflowId}` : '(save workflow first)'

  const handleSave = () => {
    const next: Trigger = { kind }
    if (kind === 'cron') next.cron_expr = cronExpr
    if (kind === 'webhook' && workflowId) next.webhook_url = `/webhooks/${workflowId}`
    setTrigger(next)
    onClose()
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60">
      <div className="bg-gray-800 border border-gray-700 rounded-xl shadow-2xl w-96 p-5">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-base font-semibold text-gray-100">Trigger Settings</h2>
          <button
            onClick={onClose}
            className="text-gray-500 hover:text-gray-300 transition-colors"
          >
            ✕
          </button>
        </div>

        {/* Kind selector */}
        <div className="flex gap-2 mb-4">
          {(['manual', 'webhook', 'cron'] as TriggerKind[]).map(k => (
            <button
              key={k}
              onClick={() => setKind(k)}
              className={`
                flex-1 py-2 rounded-lg text-sm font-medium capitalize transition-colors border
                ${kind === k
                  ? 'bg-indigo-600 border-indigo-500 text-white'
                  : 'bg-gray-700 border-gray-600 text-gray-300 hover:bg-gray-600'}
              `}
            >
              {k}
            </button>
          ))}
        </div>

        {/* Webhook info */}
        {kind === 'webhook' && (
          <div className="mb-4 rounded-md bg-gray-700/60 border border-gray-600 p-3">
            <div className="text-xs text-gray-400 mb-1">Inbound webhook URL</div>
            <div className="font-mono text-sm text-indigo-300 break-all">{webhookUrl}</div>
            <div className="text-xs text-gray-500 mt-2">
              POST JSON to this URL to trigger the workflow. The body becomes initial data.
            </div>
          </div>
        )}

        {/* Cron expression */}
        {kind === 'cron' && (
          <div className="mb-4">
            <label className="text-xs text-gray-400 block mb-1">
              Cron expression (5-field, UTC)
            </label>
            <input
              type="text"
              value={cronExpr}
              onChange={e => setCronExpr(e.target.value)}
              placeholder="* * * * *"
              className="
                w-full rounded-md bg-gray-700 border border-gray-600
                px-3 py-2 font-mono text-sm text-gray-100
                focus:outline-none focus:border-indigo-500
              "
            />
            <div className="text-xs text-gray-500 mt-1">
              min hour day month weekday — e.g. <code className="text-gray-400">0 9 * * 1-5</code>
            </div>
          </div>
        )}

        {/* Manual info */}
        {kind === 'manual' && (
          <div className="mb-4 text-sm text-gray-400">
            Workflow runs only when triggered via the "Run" button or the API.
          </div>
        )}

        <div className="flex gap-2">
          <button
            onClick={handleSave}
            className="flex-1 rounded-lg bg-indigo-600 hover:bg-indigo-500 text-white text-sm py-2 font-medium transition-colors"
          >
            Apply
          </button>
          <button
            onClick={onClose}
            className="flex-1 rounded-lg bg-gray-700 hover:bg-gray-600 text-gray-200 text-sm py-2 font-medium transition-colors"
          >
            Cancel
          </button>
        </div>
      </div>
    </div>
  )
}
