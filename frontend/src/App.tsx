import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { ErrorBoundary } from './ErrorBoundary'
import { WorkflowListPage } from './pages/WorkflowListPage'
import { WorkflowEditorPage } from './pages/WorkflowEditorPage'

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
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </BrowserRouter>
  )
}
