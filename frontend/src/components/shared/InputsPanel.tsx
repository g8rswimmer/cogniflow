import { useWorkflowStore } from '../../stores/useWorkflowStore'
import { InitialDataSchemaEditor } from '../sidebar/InitialDataSchemaEditor'

interface Props {
  onClose: () => void
}

export function InputsPanel({ onClose }: Props) {
  const initialDataSchema = useWorkflowStore(s => s.initialDataSchema)
  const fieldCount = Object.keys(
    (initialDataSchema?.properties as Record<string, unknown> | undefined) ?? {}
  ).length

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60">
      <div className="bg-gray-800 border border-gray-700 rounded-xl shadow-2xl w-[26rem] p-5">
        <div className="flex items-center justify-between mb-1">
          <h2 className="text-base font-semibold text-gray-100">Workflow Inputs</h2>
          <button
            onClick={onClose}
            className="text-gray-500 hover:text-gray-300 transition-colors"
          >
            ✕
          </button>
        </div>

        {fieldCount > 0 && (
          <p className="text-xs text-gray-500 mb-4">
            {fieldCount} field{fieldCount !== 1 ? 's' : ''} declared — referenced in nodes as{' '}
            <code className="text-indigo-300 bg-gray-900 px-1 rounded">{'{{._initial.field}}'}</code>.
          </p>
        )}

        <div className="max-h-[60vh] overflow-y-auto pr-1">
          <InitialDataSchemaEditor />
        </div>

        <div className="flex justify-end mt-4">
          <button
            onClick={onClose}
            className="
              rounded-md bg-indigo-600 hover:bg-indigo-500
              px-4 py-1.5 text-xs text-white font-semibold
              transition-colors
            "
          >
            Done
          </button>
        </div>
      </div>
    </div>
  )
}
