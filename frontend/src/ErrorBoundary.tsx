import { Component } from 'react'
import type { ReactNode } from 'react'

interface Props {
  children: ReactNode
  /** Optional label shown in the error panel header */
  label?: string
}

interface State { error: Error | null }

export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null }

  static getDerivedStateFromError(error: Error): State {
    return { error }
  }

  componentDidCatch(error: Error, info: React.ErrorInfo) {
    console.error('[ErrorBoundary]', this.props.label ?? 'app', error, info.componentStack)
  }

  reset = () => this.setState({ error: null })

  render() {
    const { error } = this.state
    if (!error) return this.props.children

    return (
      <div style={{
        fontFamily: 'monospace',
        padding: '32px',
        background: '#1a1a2e',
        color: '#e0e0e0',
        minHeight: '100%',
      }}>
        <h1 style={{ color: '#ff6b6b', fontSize: '18px', margin: '0 0 8px' }}>
          {this.props.label ? `Error in ${this.props.label}` : 'Render error'}
        </h1>
        <p style={{ color: '#ffa500', fontSize: '13px', margin: '0 0 16px' }}>
          {error.message}
        </p>
        <div style={{ display: 'flex', gap: '8px', marginBottom: '16px' }}>
          <button
            onClick={this.reset}
            style={{
              background: '#6366f1', color: '#fff', border: 'none',
              borderRadius: '6px', padding: '6px 14px', fontSize: '13px', cursor: 'pointer',
            }}
          >
            Try again
          </button>
          <button
            onClick={() => window.location.reload()}
            style={{
              background: '#374151', color: '#d1d5db', border: 'none',
              borderRadius: '6px', padding: '6px 14px', fontSize: '13px', cursor: 'pointer',
            }}
          >
            Reload page
          </button>
        </div>
        <pre style={{
          padding: '16px',
          background: '#0d0d1a',
          borderRadius: '8px',
          fontSize: '11px',
          overflowX: 'auto',
          color: '#ff9999',
          whiteSpace: 'pre-wrap',
          margin: 0,
        }}>
          {error.stack}
        </pre>
      </div>
    )
  }
}
