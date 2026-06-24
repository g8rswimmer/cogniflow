import { useEffect } from 'react'
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { ErrorBoundary } from './ErrorBoundary'
import { ProtectedRoute } from './components/shared/ProtectedRoute'
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
import { LoginPage } from './pages/LoginPage'
import { AcceptInvitePage } from './pages/AcceptInvitePage'
import { OrgUsersPage } from './pages/OrgUsersPage'
import { AdminOrgsPage } from './pages/AdminOrgsPage'
import { ToastContainer } from './components/shared/ToastContainer'
import { useAuthStore } from './stores/useAuthStore'

export default function App() {
  const initialize = useAuthStore(s => s.initialize)

  useEffect(() => {
    initialize()
  }, [initialize])

  return (
    <BrowserRouter>
      <Routes>
        {/* Public routes */}
        <Route path="/login" element={<LoginPage />} />
        <Route path="/invite/:token" element={<AcceptInvitePage />} />

        {/* Protected: any authenticated user */}
        <Route
          path="/"
          element={
            <ProtectedRoute>
              <WorkflowListPage />
            </ProtectedRoute>
          }
        />
        <Route
          path="/workflows/new"
          element={
            <ProtectedRoute>
              <ErrorBoundary label="editor">
                <WorkflowEditorPage />
              </ErrorBoundary>
            </ProtectedRoute>
          }
        />
        <Route
          path="/workflows/:id"
          element={
            <ProtectedRoute>
              <ErrorBoundary label="editor">
                <WorkflowEditorPage />
              </ErrorBoundary>
            </ProtectedRoute>
          }
        />
        <Route
          path="/workflows/:id/runs"
          element={
            <ProtectedRoute>
              <ErrorBoundary label="run history">
                <RunHistoryPage />
              </ErrorBoundary>
            </ProtectedRoute>
          }
        />
        <Route
          path="/workflows/:id/versions"
          element={
            <ProtectedRoute>
              <ErrorBoundary label="version history">
                <WorkflowVersionHistoryPage />
              </ErrorBoundary>
            </ProtectedRoute>
          }
        />
        <Route
          path="/workflows/:id/versions/:version_number"
          element={
            <ProtectedRoute>
              <ErrorBoundary label="version detail">
                <WorkflowVersionDetailPage />
              </ErrorBoundary>
            </ProtectedRoute>
          }
        />
        <Route
          path="/runs/:run_id"
          element={
            <ProtectedRoute>
              <ErrorBoundary label="run detail">
                <RunDetailPage />
              </ErrorBoundary>
            </ProtectedRoute>
          }
        />
        <Route
          path="/workflows/:id/eval-suites"
          element={
            <ProtectedRoute>
              <ErrorBoundary label="eval suites">
                <EvalSuiteListPage />
              </ErrorBoundary>
            </ProtectedRoute>
          }
        />
        <Route
          path="/eval-suites/:suite_id"
          element={
            <ProtectedRoute>
              <ErrorBoundary label="eval suite detail">
                <EvalSuiteDetailPage />
              </ErrorBoundary>
            </ProtectedRoute>
          }
        />
        <Route
          path="/eval-runs/:run_id"
          element={
            <ProtectedRoute>
              <ErrorBoundary label="eval run detail">
                <EvalRunDetailPage />
              </ErrorBoundary>
            </ProtectedRoute>
          }
        />
        <Route
          path="/admin/grader-plugins"
          element={
            <ProtectedRoute requiredRole="system_admin">
              <ErrorBoundary label="grader plugin admin">
                <GraderPluginAdminPage />
              </ErrorBoundary>
            </ProtectedRoute>
          }
        />

        {/* Protected: org_admin or higher */}
        <Route
          path="/org/users"
          element={
            <ProtectedRoute requiredRole="org_admin">
              <OrgUsersPage />
            </ProtectedRoute>
          }
        />

        {/* Protected: system_admin only */}
        <Route
          path="/admin/orgs"
          element={
            <ProtectedRoute requiredRole="system_admin">
              <AdminOrgsPage />
            </ProtectedRoute>
          }
        />

        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
      <ToastContainer />
    </BrowserRouter>
  )
}
