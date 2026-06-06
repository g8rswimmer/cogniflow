import { useEffect } from 'react'
import { useRunStore } from '../stores/useRunStore'
import type { NodeEvent } from '../api/types'

export function useRunEvents(runId: string | null) {
  const setNodeStatus = useRunStore(s => s.setNodeStatus)
  const setRunFinished = useRunStore(s => s.setRunFinished)
  const setConnectionLost = useRunStore(s => s.setConnectionLost)

  useEffect(() => {
    if (!runId) return

    const scheme = location.protocol === 'https:' ? 'wss' : 'ws'
    const ws = new WebSocket(`${scheme}://${location.host}/v1/runs/${runId}/events`)

    ws.onmessage = (evt) => {
      const event: NodeEvent = JSON.parse(evt.data as string)
      if (event.type === 'run.succeeded') {
        setRunFinished('succeeded')
        ws.close()
      } else if (event.type === 'run.failed') {
        setRunFinished('failed')
        ws.close()
      } else {
        setNodeStatus(event.node_id, event.type, event.output, event.error)
      }
    }

    // A WebSocket error means the transport failed, not that the workflow run
    // itself failed. Mark the connection as lost so the panel can direct the
    // user to check run history, without incorrectly marking the run failed.
    ws.onerror = () => {
      setConnectionLost()
    }

    return () => {
      if (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING) {
        ws.close()
      }
    }
  }, [runId, setNodeStatus, setRunFinished, setConnectionLost])
}
