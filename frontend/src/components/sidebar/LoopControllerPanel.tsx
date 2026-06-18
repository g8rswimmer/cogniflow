interface Props {
  nodeId: string
  config: Record<string, unknown>
  onChange: (data: Record<string, unknown>) => void
  fieldErrors?: Record<string, string>
}

export function LoopControllerPanel({ config, onChange, fieldErrors = {} }: Props) {
  const maxIter = (config.max_iterations as number | undefined) ?? 10
  const exitCondition = (config.exit_condition as string | undefined) ?? ''

  const update = (patch: Record<string, unknown>) => {
    onChange({ ...config, ...patch })
  }

  return (
    <div className="space-y-4">
      <div>
        <label className="text-xs font-semibold uppercase tracking-wider text-gray-400 block mb-2">
          Loop Configuration
        </label>

        {/* Max iterations */}
        <div className="mb-3">
          <label className="block text-xs text-gray-300 mb-1">
            Max Iterations <span className="text-red-400">*</span>
          </label>
          <input
            type="number"
            min={1}
            max={100}
            value={maxIter}
            onChange={e => update({ max_iterations: parseInt(e.target.value, 10) || 1 })}
            className="w-full bg-gray-800 border border-gray-600 rounded px-2 py-1.5 text-sm text-gray-100 focus:outline-none focus:border-indigo-500"
          />
          {fieldErrors.max_iterations && (
            <p className="text-xs text-red-400 mt-1">{fieldErrors.max_iterations}</p>
          )}
          <p className="text-xs text-gray-500 mt-1">
            Hard cap on iterations (1–100). The loop exits gracefully when reached.
          </p>
        </div>

        {/* Exit condition */}
        <div>
          <label className="block text-xs text-gray-300 mb-1">
            Exit Condition (CEL) <span className="text-gray-500">optional</span>
          </label>
          <textarea
            rows={3}
            value={exitCondition}
            onChange={e => update({ exit_condition: e.target.value })}
            placeholder={`ctx["body_node"]["done"] == true`}
            className="w-full bg-gray-800 border border-gray-600 rounded px-2 py-1.5 text-sm text-gray-100 font-mono focus:outline-none focus:border-indigo-500 resize-none"
          />
          {fieldErrors.exit_condition && (
            <p className="text-xs text-red-400 mt-1">{fieldErrors.exit_condition}</p>
          )}
          <p className="text-xs text-gray-500 mt-1">
            CEL expression evaluated against upstream data each iteration. Loop exits early when true.
            Use <code className="text-amber-400">ctx["nodeId"]["field"]</code> to reference node outputs.
          </p>
        </div>
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
