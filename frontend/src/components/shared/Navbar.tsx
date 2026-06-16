import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useWorkflowStore } from '../../stores/useWorkflowStore'
import { WorkflowSettingsPanel } from './WorkflowSettingsPanel'

interface Props {
  onSave: () => void
  onRun: () => void
  saving: boolean
  running: boolean
}

const triggerIcons: Record<string, string> = {
  manual: '▶',
  webhook: '⚡',
  cron: '⏰',
}

export function Navbar({ onSave, onRun, saving, running }: Props) {
  const name = useWorkflowStore(s => s.name)
  const setName = useWorkflowStore(s => s.setName)
  const isDirty = useWorkflowStore(s => s.isDirty)
  const workflowId = useWorkflowStore(s => s.workflowId)
  const trigger = useWorkflowStore(s => s.trigger)

  const [showSettings, setShowSettings] = useState(false)

  return (
    <>
      <header className="h-12 flex items-center px-3 gap-3 bg-gray-900 border-b border-gray-700 flex-shrink-0">
        {/* Back link */}
        <Link
          to="/"
          className="text-gray-400 hover:text-gray-200 transition-colors flex-shrink-0 text-sm"
          title="All workflows"
        >
          ← Workflows
        </Link>

        <div className="w-px h-5 bg-gray-700" />

        {/* Editable workflow name */}
        <input
          type="text"
          value={name}
          onChange={e => setName(e.target.value)}
          className="
            flex-1 min-w-0 bg-transparent text-sm font-semibold text-gray-100
            border-b border-transparent hover:border-gray-600 focus:border-indigo-500
            focus:outline-none px-1 py-0.5 transition-colors
          "
          placeholder="Workflow name"
        />

        {/* Dirty indicator */}
        {isDirty && (
          <span className="text-xs text-amber-400 flex-shrink-0">unsaved</span>
        )}

        {/* Settings button */}
        <button
          onClick={() => setShowSettings(true)}
          className="
            flex items-center gap-1.5 rounded-md border border-gray-600
            bg-gray-700 hover:bg-gray-600 text-gray-200 px-2.5 py-1.5
            text-xs font-medium transition-colors flex-shrink-0
          "
          title="Workflow settings — title, description, trigger, inputs"
        >
          <span>{triggerIcons[trigger.kind] ?? '▶'}</span>
          <span>Settings</span>
        </button>

        {/* Eval Suites link — shown only for saved workflows */}
        {workflowId && (
          <Link
            to={`/workflows/${workflowId}/eval-suites`}
            className="
              flex items-center gap-1.5 rounded-md border border-gray-600
              bg-gray-700 hover:bg-gray-600 text-gray-200 px-2.5 py-1.5
              text-xs font-medium transition-colors flex-shrink-0
            "
            title="Eval suites for this workflow"
          >
            <span>⚗</span>
            <span>Evals</span>
          </Link>
        )}

        {/* Run button — enabled only when workflow is saved */}
        <button
          onClick={onRun}
          disabled={running || !workflowId}
          title={!workflowId ? 'Save the workflow first' : 'Trigger a manual run'}
          className="
            rounded-md bg-green-700 hover:bg-green-600 disabled:opacity-40
            text-white text-xs font-semibold px-3 py-1.5 transition-colors flex-shrink-0
            flex items-center gap-1.5
          "
        >
          {running ? '…' : '▶'} {running ? 'Running' : 'Run'}
        </button>

        {/* Save button */}
        <button
          onClick={onSave}
          disabled={saving}
          className="
            rounded-md bg-indigo-600 hover:bg-indigo-500 disabled:opacity-50
            text-white text-xs font-semibold px-3 py-1.5 transition-colors flex-shrink-0
          "
        >
          {saving ? 'Saving…' : 'Save'}
        </button>
      </header>

      {showSettings && (
        <WorkflowSettingsPanel
          workflowId={workflowId}
          onClose={() => setShowSettings(false)}
        />
      )}
    </>
  )
}
