import { useEffect, useState } from 'react'
import type { EvalEvent, EvalRunSummary, TestCaseResult } from '../api/types'

interface EvalRunEventState {
  // Results accumulated from eval.test_case.completed events, keyed by test_case_id.
  // Populated progressively as each test case finishes.
  liveResults: TestCaseResult[]
  // Final aggregate counts from the terminal eval.run.completed event.
  summary: EvalRunSummary | null
  // True once eval.run.completed or eval.run.failed has been received.
  isTerminal: boolean
  // True while the WebSocket connection is open.
  isConnected: boolean
  // True if the connection closed before a terminal event was received.
  // The page should fall back to polling when this is set.
  connectionLost: boolean
}

// useEvalRunEvents connects to GET /v1/eval-runs/{evalRunId}/events and streams
// EvalEvents for the given eval run. Pass null to skip connecting (e.g. while the
// run ID is still loading).
//
// For already-terminal runs the backend sends a fast-path burst of synthetic events
// (one per stored TestCaseResult, then the terminal summary), so the hook works
// identically for live and historical runs.
export function useEvalRunEvents(evalRunId: string | null): EvalRunEventState {
  const [liveResults, setLiveResults] = useState<TestCaseResult[]>([])
  const [summary, setSummary] = useState<EvalRunSummary | null>(null)
  const [isTerminal, setIsTerminal] = useState(false)
  const [isConnected, setIsConnected] = useState(false)
  const [connectionLost, setConnectionLost] = useState(false)

  useEffect(() => {
    if (!evalRunId) return

    setLiveResults([])
    setSummary(null)
    setIsTerminal(false)
    setIsConnected(false)
    setConnectionLost(false)

    // terminalReceived is a closure-local flag so onclose can check it without
    // depending on the isTerminal React state (which may not have flushed yet).
    let terminalReceived = false

    const scheme = location.protocol === 'https:' ? 'wss' : 'ws'
    const ws = new WebSocket(`${scheme}://${location.host}/v1/eval-runs/${evalRunId}/events`)

    ws.onopen = () => setIsConnected(true)

    ws.onmessage = (evt) => {
      const event: EvalEvent = JSON.parse(evt.data as string)

      if (event.type === 'eval.test_case.completed' && event.result) {
        const result = event.result
        setLiveResults(prev => {
          // Upsert by test_case_id so reconnects don't duplicate rows.
          const idx = prev.findIndex(r => r.test_case_id === result.test_case_id)
          if (idx >= 0) {
            const next = [...prev]
            next[idx] = result
            return next
          }
          return [...prev, result]
        })
      } else if (event.type === 'eval.run.completed' || event.type === 'eval.run.failed') {
        terminalReceived = true
        if (event.summary) setSummary(event.summary)
        setIsTerminal(true)
        ws.close()
      }
    }

    ws.onerror = () => setIsConnected(false)

    ws.onclose = () => {
      setIsConnected(false)
      // If the connection dropped before we received a terminal event, signal the
      // page so it can fall back to polling rather than staying stuck.
      if (!terminalReceived) {
        setConnectionLost(true)
      }
    }

    return () => {
      if (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING) {
        ws.close()
      }
    }
  }, [evalRunId])

  return { liveResults, summary, isTerminal, isConnected, connectionLost }
}
