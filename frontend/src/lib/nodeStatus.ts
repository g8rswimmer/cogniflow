export type NodeState = 'pending' | 'running' | 'succeeded' | 'failed'

export const nodeStatusDot: Record<NodeState, string> = {
  pending: 'bg-gray-500',
  running: 'bg-amber-400 animate-pulse',
  succeeded: 'bg-green-400',
  failed: 'bg-red-400',
}

export const nodeStatusRing: Record<NodeState, string> = {
  pending: 'ring-gray-500',
  running: 'ring-amber-400',
  succeeded: 'ring-green-400',
  failed: 'ring-red-500',
}

export const nodeStatusLabel: Record<NodeState, string> = {
  pending: 'pending',
  running: 'running',
  succeeded: 'succeeded',
  failed: 'failed',
}
