import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { ErrorBoundary } from './ErrorBoundary'
import { WorkflowListPage } from './pages/WorkflowListPage'
import { WorkflowEditorPage } from './pages/WorkflowEditorPage'
import { RunHistoryPage } from './pages/RunHistoryPage'
import { RunDetailPage } from './pages/RunDetailPage'

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/" element={<WorkflowListPage />} />
        <Route
          path="/workflows/new"
          element={
            <ErrorBoundary label="editor">
              <WorkflowEditorPage />
            </ErrorBoundary>
          }
        />
        <Route
          path="/workflows/:id"
          element={
            <ErrorBoundary label="editor">
              <WorkflowEditorPage />
            </ErrorBoundary>
          }
        />
        <Route
          path="/workflows/:id/runs"
          element={
            <ErrorBoundary label="run history">
              <RunHistoryPage />
            </ErrorBoundary>
          }
        />
        <Route
          path="/runs/:run_id"
          element={
            <ErrorBoundary label="run detail">
              <RunDetailPage />
            </ErrorBoundary>
          }
        />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </BrowserRouter>
  )
}
