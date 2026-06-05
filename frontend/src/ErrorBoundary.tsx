import { Component } from 'react'
import type { ReactNode } from 'react'

interface Props { children: ReactNode }
interface State { error: Error | null }

export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null }

  static getDerivedStateFromError(error: Error): State {
    return { error }
  }

  render() {
    const { error } = this.state
    if (!error) return this.props.children

    return (
      <div style={{
        fontFamily: 'monospace',
        padding: '32px',
        background: '#1a1a2e',
        color: '#e0e0e0',
        minHeight: '100vh',
      }}>
        <h1 style={{ color: '#ff6b6b', fontSize: '20px' }}>React render error</h1>
        <p style={{ color: '#ffa500', fontSize: '14px', marginTop: '8px' }}>
          {error.message}
        </p>
        <pre style={{
          marginTop: '16px',
          padding: '16px',
          background: '#0d0d1a',
          borderRadius: '8px',
          fontSize: '12px',
          overflowX: 'auto',
          color: '#ff9999',
          whiteSpace: 'pre-wrap',
        }}>
          {error.stack}
        </pre>
      </div>
    )
  }
}
