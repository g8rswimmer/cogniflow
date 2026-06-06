import { useCallback } from 'react'
import { Panel, useReactFlow, useStore, useStoreApi } from '@xyflow/react'

// ---------------------------------------------------------------------------
// Individual button
// ---------------------------------------------------------------------------

interface BtnProps {
  onClick: () => void
  title: string
  active?: boolean
  children: React.ReactNode
}

function CtrlBtn({ onClick, title, active, children }: BtnProps) {
  return (
    <button
      onClick={onClick}
      title={title}
      aria-label={title}
      className={[
        'w-8 h-8 flex items-center justify-center transition-colors',
        active
          ? 'bg-indigo-700 text-white'
          : 'bg-gray-800 hover:bg-gray-700 text-gray-300 hover:text-white',
      ].join(' ')}
    >
      {children}
    </button>
  )
}

// ---------------------------------------------------------------------------
// SVG icons (stroke-based, 24 × 24 viewBox)
// ---------------------------------------------------------------------------

function IconPlus() {
  return (
    <svg viewBox="0 0 24 24" className="w-4 h-4" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round">
      <path d="M12 5v14M5 12h14" />
    </svg>
  )
}

function IconMinus() {
  return (
    <svg viewBox="0 0 24 24" className="w-4 h-4" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round">
      <path d="M5 12h14" />
    </svg>
  )
}

function IconFit() {
  return (
    <svg viewBox="0 0 24 24" className="w-4 h-4" fill="none" stroke="currentColor" strokeWidth={1.75} strokeLinecap="round" strokeLinejoin="round">
      <path d="M4 9V4m0 0h5M4 4l6 6M20 9V4m0 0h-5m5 0l-6 6M4 15v5m0 0h5M4 20l6-6M20 15v5m0 0h-5m5 0l-6-6" />
    </svg>
  )
}

function IconLocked() {
  return (
    <svg viewBox="0 0 24 24" className="w-4 h-4" fill="none" stroke="currentColor" strokeWidth={1.75} strokeLinecap="round" strokeLinejoin="round">
      <rect x="3" y="11" width="18" height="11" rx="2" />
      <path d="M7 11V7a5 5 0 0110 0v4" />
    </svg>
  )
}

function IconUnlocked() {
  return (
    <svg viewBox="0 0 24 24" className="w-4 h-4" fill="none" stroke="currentColor" strokeWidth={1.75} strokeLinecap="round" strokeLinejoin="round">
      <rect x="3" y="11" width="18" height="11" rx="2" />
      <path d="M7 11V7a5 5 0 019.9-1" />
    </svg>
  )
}

// ---------------------------------------------------------------------------
// Control panel
// ---------------------------------------------------------------------------

interface Props {
  showInteractive?: boolean
}

export function CanvasControls({ showInteractive = true }: Props) {
  const { zoomIn, zoomOut, fitView } = useReactFlow()
  const store = useStoreApi()
  const isInteractive = useStore(
    s => s.nodesDraggable || s.nodesConnectable || s.elementsSelectable,
  )

  const toggleInteractive = useCallback(() => {
    store.setState({
      nodesDraggable: !isInteractive,
      nodesConnectable: !isInteractive,
      elementsSelectable: !isInteractive,
    })
  }, [store, isInteractive])

  return (
    <Panel position="bottom-left" className="m-3">
      <div className="flex flex-col rounded-lg overflow-hidden shadow-xl border border-gray-600 divide-y divide-gray-600">
        <CtrlBtn onClick={() => zoomIn()} title="Zoom in">
          <IconPlus />
        </CtrlBtn>
        <CtrlBtn onClick={() => zoomOut()} title="Zoom out">
          <IconMinus />
        </CtrlBtn>
        <CtrlBtn onClick={() => fitView({ padding: 0.15 })} title="Fit view">
          <IconFit />
        </CtrlBtn>
        {showInteractive && (
          <CtrlBtn
            onClick={toggleInteractive}
            title={isInteractive ? 'Lock canvas — disable drag and connect' : 'Unlock canvas — enable drag and connect'}
            active={!isInteractive}
          >
            {isInteractive ? <IconUnlocked /> : <IconLocked />}
          </CtrlBtn>
        )}
      </div>
    </Panel>
  )
}
