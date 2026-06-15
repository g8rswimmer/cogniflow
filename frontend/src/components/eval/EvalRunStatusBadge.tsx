import type { EvalRunStatus } from '../../api/types'

const colors: Record<EvalRunStatus, string> = {
  pending: 'bg-gray-600 text-gray-300',
  running: 'bg-amber-700 text-amber-200',
  completed: 'bg-green-700 text-green-200',
  failed: 'bg-red-700 text-red-200',
}

interface Props {
  status: string
  size?: 'sm' | 'md'
}

export function EvalRunStatusBadge({ status, size = 'md' }: Props) {
  const padding = size === 'sm' ? 'px-1.5 py-0.5' : 'px-2 py-0.5'
  return (
    <span className={`${padding} rounded text-xs font-semibold ${colors[status as EvalRunStatus] ?? 'bg-gray-600 text-gray-300'}`}>
      {status}
    </span>
  )
}
