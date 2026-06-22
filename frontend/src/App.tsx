import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { ErrorBoundary } from './ErrorBoundary'
import { WorkflowListPage } from './pages/WorkflowListPage'
import { WorkflowEditorPage } from './pages/WorkflowEditorPage'
import { RunHistoryPage } from './pages/RunHistoryPage'
import { RunDetailPage } from './pages/RunDetailPage'
import { WorkflowVersionHistoryPage } from './pages/WorkflowVersionHistoryPage'
import { WorkflowVersionDetailPage } from './pages/WorkflowVersionDetailPage'
import { EvalSuiteListPage } from './pages/EvalSuiteListPage'
import { EvalSuiteDetailPage } from './pages/EvalSuiteDetailPage'
import { EvalRunDetailPage } from './pages/EvalRunDetailPage'
import { GraderPluginAdminPage } from './pages/GraderPluginAdminPage'
import { ToastContainer } from './components/shared/ToastContainer'

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
          path="/workflows/:id/versions"
          element={
            <ErrorBoundary label="version history">
              <WorkflowVersionHistoryPage />
            </ErrorBoundary>
          }
        />
        <Route
          path="/workflows/:id/versions/:version_number"
          element={
            <ErrorBoundary label="version detail">
              <WorkflowVersionDetailPage />
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
        <Route
          path="/workflows/:id/eval-suites"
          element={
            <ErrorBoundary label="eval suites">
              <EvalSuiteListPage />
            </ErrorBoundary>
          }
        />
        <Route
          path="/eval-suites/:suite_id"
          element={
            <ErrorBoundary label="eval suite detail">
              <EvalSuiteDetailPage />
            </ErrorBoundary>
          }
        />
        <Route
          path="/eval-runs/:run_id"
          element={
            <ErrorBoundary label="eval run detail">
              <EvalRunDetailPage />
            </ErrorBoundary>
          }
        />
        <Route
          path="/admin/grader-plugins"
          element={
            <ErrorBoundary label="grader plugin admin">
              <GraderPluginAdminPage />
            </ErrorBoundary>
          }
        />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
      <ToastContainer />
    </BrowserRouter>
  )
}
