import type { RunStatus } from '../../api/types'

const statusColors: Record<RunStatus, string> = {
  pending: 'bg-gray-600 text-gray-300',
  running: 'bg-amber-700 text-amber-200',
  succeeded: 'bg-green-700 text-green-200',
  failed: 'bg-red-700 text-red-200',
}

interface Props {
  status: RunStatus
}

export function StatusBadge({ status }: Props) {
  return (
    <span className={`px-2 py-0.5 rounded text-xs font-semibold ${statusColors[status] ?? 'bg-gray-600 text-gray-300'}`}>
      {status}
    </span>
  )
}
