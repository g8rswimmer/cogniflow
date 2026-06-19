import { ExitConditionBuilder } from './ExitConditionBuilder'

interface Props {
  nodeId: string
  config: Record<string, unknown>
  onChange: (data: Record<string, unknown>) => void
  fieldErrors?: Record<string, string>
}

export function LoopControllerPanel({ nodeId, config, onChange, fieldErrors = {} }: Props) {
  const maxIter = (config.max_iterations as number | undefined) ?? 10

  return (
    <div className="space-y-4">
      {/* Max iterations */}
      <div>
        <label className="text-xs font-semibold uppercase tracking-wider text-gray-400 block mb-2">
          Loop Configuration
        </label>
        <div className="mb-3">
          <label className="block text-xs text-gray-300 mb-1">
            Max Iterations <span className="text-red-400">*</span>
          </label>
          <input
            type="number"
            min={1}
            max={100}
            value={maxIter}
            onChange={e =>
              onChange({ ...config, max_iterations: parseInt(e.target.value, 10) || 1 })
            }
            className="w-full bg-gray-800 border border-gray-600 rounded px-2 py-1.5 text-sm text-gray-100 focus:outline-none focus:border-indigo-500"
          />
          {fieldErrors.max_iterations && (
            <p className="text-xs text-red-400 mt-1">{fieldErrors.max_iterations}</p>
          )}
          <p className="text-xs text-gray-500 mt-1">
            Hard cap on iterations (1–100). The loop exits gracefully when reached.
          </p>
        </div>
      </div>

      {/* Exit condition builder */}
      <div>
        <label className="text-xs font-semibold uppercase tracking-wider text-gray-400 block mb-2">
          Exit Condition <span className="text-gray-600 font-normal normal-case">(optional)</span>
        </label>
        <p className="text-[10px] text-gray-500 mb-2">
          Loop exits early when all conditions match. Leave empty to always run max iterations.
        </p>
        <ExitConditionBuilder
          nodeId={nodeId}
          config={config}
          onChange={onChange}
          fieldErrors={fieldErrors}
        />
      </div>

      {/* Wiring guide */}
      <div className="rounded border border-amber-700/40 bg-amber-900/20 p-3 text-xs text-amber-300 space-y-1">
        <p className="font-semibold">Wiring this node:</p>
        <ol className="list-decimal list-inside space-y-0.5 text-amber-200/80">
          <li>Connect a <strong>loop_body</strong> edge to the first node inside the loop.</li>
          <li>Connect an <strong>exit</strong> edge to the first node after the loop.</li>
          <li>Draw an edge from the last body node <em>back</em> to this controller — it will be automatically marked as a loop-back edge (shown in amber).</li>
        </ol>
      </div>
    </div>
  )
}
