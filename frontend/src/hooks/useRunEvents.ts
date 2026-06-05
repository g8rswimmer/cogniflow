import { useEffect } from 'react'
import { useRunStore } from '../stores/useRunStore'
import type { NodeEvent } from '../api/types'

export function useRunEvents(runId: string | null) {
  const setNodeStatus = useRunStore(s => s.setNodeStatus)
  const setRunFinished = useRunStore(s => s.setRunFinished)

  useEffect(() => {
    if (!runId) return

    const ws = new WebSocket(`ws://${location.host}/v1/runs/${runId}/events`)

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

    ws.onerror = () => {
      setRunFinished('failed')
    }

    return () => {
      if (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING) {
        ws.close()
      }
    }
  }, [runId, setNodeStatus, setRunFinished])
}
