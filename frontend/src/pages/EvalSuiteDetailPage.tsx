import { useEffect, useState, useCallback } from 'react'
import { useParams, Link, useNavigate } from 'react-router-dom'
import { api } from '../hooks/useApi'
import { useEvalStore } from '../stores/useEvalStore'
import { EvalSuiteForm } from '../components/eval/EvalSuiteForm'
import { TestCaseList } from '../components/eval/TestCaseList'
import { TestCaseEditor } from '../components/eval/TestCaseEditor'
import { ApiError } from '../api/client'
import type { EvalSuite, TestCase, EvalRun } from '../api/types'
import type { NodeOption } from '../components/eval/MockEditor'

function EvalRunHistoryPanel({ suiteId }: { suiteId: string }) {
  const [runs, setRuns] = useState<EvalRun[]>([])
  const [loading, setLoading] = useState(true)
  const [open, setOpen] = useState(false)
  const navigate = useNavigate()

  useEffect(() => {
    if (!open) return
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setLoading(true)
    api.listEvalRuns(suiteId)
      .then(r => setRuns(r.eval_runs ?? []))
      .catch(() => setRuns([]))
      .finally(() => setLoading(false))
  }, [suiteId, open])

  return (
    <div className="border border-gray-700 rounded-lg overflow-hidden">
      <button
        type="button"
        onClick={() => setOpen(o => !o)}
        className="w-full flex items-center justify-between px-4 py-2.5 bg-gray-800 hover:bg-gray-750 transition-colors text-left"
      >
        <span className="text-xs font-semibold text-gray-400 uppercase tracking-wide">Run History</span>
        <span className="text-gray-500 text-xs">{open ? '▾' : '▸'}</span>
      </button>

      {open && (
        <div className="border-t border-gray-700 bg-gray-900">
          {loading ? (
            <p className="text-xs text-gray-500 px-4 py-3">Loading…</p>
          ) : runs.length === 0 ? (
            <p className="text-xs text-gray-600 italic px-4 py-3">No runs yet.</p>
          ) : (
            <div className="divide-y divide-gray-800">
              {runs.map(run => (
                <div
                  key={run.id}
                  className="flex items-center justify-between px-4 py-2.5 hover:bg-gray-800 transition-colors cursor-pointer"
                  onClick={() => navigate(`/eval-runs/${run.id}`)}
                >
                  <div>
                    <span className="text-xs font-mono text-gray-400">{run.id.slice(0, 8)}…</span>
                    <span className="text-xs text-gray-600 ml-2">
                      {run.started_at ? new Date(run.started_at).toLocaleString() : new Date(run.created_at).toLocaleString()}
                    </span>
                  </div>
                  <div className="flex items-center gap-2">
                    <span className="text-xs text-gray-400">
                      {run.passed_count}/{run.total_cases} passed
                    </span>
                    <RunStatusBadge status={run.status} />
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  )
}

function RunStatusBadge({ status }: { status: string }) {
  const colors: Record<string, string> = {
    pending: 'bg-gray-600 text-gray-300',
    running: 'bg-amber-700 text-amber-200',
    completed: 'bg-green-700 text-green-200',
    failed: 'bg-red-700 text-red-200',
  }
  return (
    <span className={`px-1.5 py-0.5 rounded text-xs font-semibold ${colors[status] ?? 'bg-gray-600 text-gray-300'}`}>
      {status}
    </span>
  )
}

export function EvalSuiteDetailPage() {
  const { suite_id: suiteId } = useParams<{ suite_id: string }>()
  const navigate = useNavigate()

  const activeSuite = useEvalStore(s => s.activeSuite)
  const setActiveSuite = useEvalStore(s => s.setActiveSuite)
  const testCases = useEvalStore(s => s.testCases)
  const setTestCases = useEvalStore(s => s.setTestCases)
  const upsertTestCase = useEvalStore(s => s.upsertTestCase)
  const removeTestCase = useEvalStore(s => s.removeTestCase)
  const upsertSuite = useEvalStore(s => s.upsertSuite)

  const [workflowNodes, setWorkflowNodes] = useState<NodeOption[]>([])
  const [initialDataSchema, setInitialDataSchema] = useState<Record<string, unknown> | null>(null)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)

  const [showSuiteForm, setShowSuiteForm] = useState(false)
  const [suiteFormSaving, setSuiteFormSaving] = useState(false)
  const [suiteFormError, setSuiteFormError] = useState<string | undefined>()

  const [editorOpen, setEditorOpen] = useState(false)
  const [editingCase, setEditingCase] = useState<TestCase | null>(null)
  const [editorSaving, setEditorSaving] = useState(false)
  const [editorErrors, setEditorErrors] = useState<{ field?: string; message: string }[]>([])
  const [deleting, setDeleting] = useState<string | null>(null)
  const [triggeringRun, setTriggeringRun] = useState(false)

  const loadData = useCallback(async () => {
    if (!suiteId) return
    setLoading(true)
    setLoadError(null)
    try {
      const [suite, tcResp] = await Promise.all([
        api.getEvalSuite(suiteId),
        api.listTestCases(suiteId),
      ])
      setActiveSuite(suite)
      const sorted = (tcResp.test_cases ?? []).sort((a, b) => a.position - b.position)
      setTestCases(sorted)
      // Fetch the workflow for nodes and schema
      try {
        const wf = await api.getWorkflow(suite.workflow_id)
        const nodeOptions: NodeOption[] = (wf.nodes ?? []).map(n => ({
          id: n.id,
          label: n.label,
        }))
        console.debug('[EvalSuiteDetailPage] workflow nodes loaded:', nodeOptions)
        setWorkflowNodes(nodeOptions)
        setInitialDataSchema(wf.initial_data_schema ?? null)
      } catch {
        // Non-fatal: workflow may have been deleted
      }
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : 'Failed to load suite')
    } finally {
      setLoading(false)
    }
  }, [suiteId, setActiveSuite, setTestCases])

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    loadData()
  }, [loadData])

  const handleSuiteUpdate = async (data: {
    name: string
    description?: string
    pass_threshold: number
    max_concurrency: number
  }) => {
    if (!suiteId) return
    setSuiteFormSaving(true)
    setSuiteFormError(undefined)
    try {
      const updated = await api.updateEvalSuite(suiteId, data)
      setActiveSuite(updated)
      upsertSuite(updated)
      setShowSuiteForm(false)
    } catch (err) {
      setSuiteFormError(err instanceof Error ? err.message : 'Failed to update suite')
    } finally {
      setSuiteFormSaving(false)
    }
  }

  const openEditor = (tc?: TestCase) => {
    setEditingCase(tc ?? null)
    setEditorErrors([])
    setEditorOpen(true)
  }

  const handleSaveTestCase = async (
    data: Omit<TestCase, 'id' | 'suite_id' | 'position' | 'created_at' | 'updated_at'>
  ) => {
    if (!suiteId) return
    setEditorSaving(true)
    setEditorErrors([])
    try {
      if (editingCase) {
        const updated = await api.updateTestCase(suiteId, editingCase.id, data)
        upsertTestCase(updated)
      } else {
        const created = await api.createTestCase(suiteId, data)
        upsertTestCase(created)
      }
      setEditorOpen(false)
      setEditingCase(null)
    } catch (err) {
      if (err instanceof ApiError && err.validationErrors.length > 0) {
        setEditorErrors(err.validationErrors)
      } else {
        setEditorErrors([{ message: err instanceof Error ? err.message : 'Save failed' }])
      }
    } finally {
      setEditorSaving(false)
    }
  }

  const handleDeleteTestCase = async (tc: TestCase) => {
    if (!suiteId) return
    if (!confirm(`Delete test case "${tc.name}"?`)) return
    setDeleting(tc.id)
    try {
      await api.deleteTestCase(suiteId, tc.id)
      removeTestCase(tc.id)
    } catch (err) {
      alert(err instanceof Error ? err.message : 'Delete failed')
    } finally {
      setDeleting(null)
    }
  }

  const handleMoveUp = async (tc: TestCase) => {
    const idx = testCases.findIndex(x => x.id === tc.id)
    if (idx <= 0) return
    const reordered = [...testCases]
    ;[reordered[idx - 1], reordered[idx]] = [reordered[idx], reordered[idx - 1]]
    const ids = reordered.map(x => x.id)
    try {
      await api.reorderTestCases(suiteId!, ids)
      setTestCases(reordered.map((x, i) => ({ ...x, position: i })))
    } catch {
      // Refresh from server on failure
      loadData()
    }
  }

  const handleMoveDown = async (tc: TestCase) => {
    const idx = testCases.findIndex(x => x.id === tc.id)
    if (idx < 0 || idx >= testCases.length - 1) return
    const reordered = [...testCases]
    ;[reordered[idx], reordered[idx + 1]] = [reordered[idx + 1], reordered[idx]]
    const ids = reordered.map(x => x.id)
    try {
      await api.reorderTestCases(suiteId!, ids)
      setTestCases(reordered.map((x, i) => ({ ...x, position: i })))
    } catch {
      loadData()
    }
  }

  const handleRunSuite = async () => {
    if (!suiteId) return
    setTriggeringRun(true)
    try {
      const run = await api.triggerEvalRun(suiteId)
      navigate(`/eval-runs/${run.id}`)
    } catch (err) {
      alert(err instanceof Error ? err.message : 'Failed to start run')
      setTriggeringRun(false)
    }
  }

  const handleDeleteSuite = async () => {
    if (!activeSuite) return
    if (!confirm(`Delete suite "${activeSuite.name}" and all its data? This cannot be undone.`)) return
    try {
      await api.deleteEvalSuite(activeSuite.id)
      navigate(`/workflows/${activeSuite.workflow_id}/eval-suites`)
    } catch (err) {
      alert(err instanceof Error ? err.message : 'Delete failed')
    }
  }

  if (loading) {
    return (
      <div className="min-h-screen bg-gray-950 flex items-center justify-center">
        <p className="text-gray-400 text-sm">Loading…</p>
      </div>
    )
  }

  if (loadError) {
    return (
      <div className="min-h-screen bg-gray-950 flex items-center justify-center">
        <div className="text-center">
          <p className="text-red-400 mb-4">{loadError}</p>
          <button onClick={() => navigate(-1)} className="text-sm text-indigo-400 hover:text-indigo-300">
            ← Go back
          </button>
        </div>
      </div>
    )
  }

  if (!activeSuite) return null

  return (
    <div className="min-h-screen bg-gray-950 text-gray-100">
      <div className="max-w-4xl mx-auto px-4 py-8">
        {/* Breadcrumb */}
        <div className="flex items-center gap-2 text-sm mb-6">
          <Link
            to={`/workflows/${activeSuite.workflow_id}/eval-suites`}
            className="text-indigo-400 hover:text-indigo-300 transition-colors"
          >
            ← Eval Suites
          </Link>
        </div>

        {/* Suite header */}
        <div className="rounded-lg bg-gray-800 border border-gray-700 px-5 py-4 mb-6">
          <div className="flex items-start justify-between gap-4">
            <div className="min-w-0">
              <div className="flex items-center gap-2">
                <h1 className="text-lg font-bold text-gray-100">{activeSuite.name}</h1>
                {activeSuite.workflow_deleted && (
                  <span className="text-xs px-1.5 py-0.5 rounded bg-amber-900/60 text-amber-300">
                    workflow deleted
                  </span>
                )}
              </div>
              {activeSuite.description && (
                <p className="text-sm text-gray-400 mt-1">{activeSuite.description}</p>
              )}
              <div className="flex items-center gap-4 mt-2">
                <span className="text-xs text-gray-500">
                  Pass threshold: <span className="text-gray-300 font-medium">{Math.round(activeSuite.pass_threshold * 100)}%</span>
                </span>
                <span className="text-xs text-gray-500">
                  Concurrency: <span className="text-gray-300 font-medium">{activeSuite.max_concurrency ?? 1}</span>
                </span>
                <span className="text-xs text-gray-600">
                  Updated {new Date(activeSuite.updated_at).toLocaleString()}
                </span>
              </div>
            </div>
            <div className="flex gap-2 flex-shrink-0">
              <button
                type="button"
                onClick={() => setShowSuiteForm(true)}
                className="rounded-md border border-gray-600 bg-gray-700 hover:bg-gray-600 text-gray-200 px-3 py-1.5 text-xs font-medium transition-colors"
              >
                Edit
              </button>
              <button
                type="button"
                onClick={handleDeleteSuite}
                className="rounded-md border border-red-900 bg-red-900/30 hover:bg-red-900/50 text-red-400 px-3 py-1.5 text-xs font-medium transition-colors"
              >
                Delete
              </button>
              <button
                type="button"
                onClick={handleRunSuite}
                disabled={triggeringRun || !!activeSuite.workflow_deleted}
                title={activeSuite.workflow_deleted ? 'Workflow has been deleted' : 'Run all test cases'}
                className="rounded-md bg-green-700 hover:bg-green-600 disabled:opacity-40 text-white px-3 py-1.5 text-xs font-semibold transition-colors flex items-center gap-1"
              >
                {triggeringRun ? '…' : '▶'} {triggeringRun ? 'Starting…' : 'Run Suite'}
              </button>
            </div>
          </div>
        </div>

        {/* Test cases */}
        <div className="mb-6">
          <div className="flex items-center justify-between mb-3">
            <h2 className="text-sm font-semibold text-gray-300">
              Test Cases <span className="text-gray-600 font-normal">({testCases.length})</span>
            </h2>
            <button
              type="button"
              onClick={() => openEditor()}
              className="rounded-md bg-indigo-600 hover:bg-indigo-500 text-white text-xs font-semibold px-3 py-1.5 transition-colors"
            >
              + Add Test Case
            </button>
          </div>

          <TestCaseList
            testCases={testCases}
            onEdit={tc => openEditor(tc)}
            onDelete={handleDeleteTestCase}
            onMoveUp={handleMoveUp}
            onMoveDown={handleMoveDown}
            deleting={deleting}
          />
        </div>

        {/* Run history */}
        <EvalRunHistoryPanel suiteId={activeSuite.id} />
      </div>

      {/* Suite edit form */}
      {showSuiteForm && (
        <EvalSuiteForm
          suite={activeSuite as EvalSuite}
          onSave={handleSuiteUpdate}
          onClose={() => { setShowSuiteForm(false); setSuiteFormError(undefined) }}
          saving={suiteFormSaving}
          error={suiteFormError}
        />
      )}

      {/* Test case editor */}
      {editorOpen && (
        <TestCaseEditor
          testCase={editingCase ?? undefined}
          nodes={workflowNodes}
          initialDataSchema={initialDataSchema}
          onSave={handleSaveTestCase}
          onClose={() => { setEditorOpen(false); setEditingCase(null) }}
          saving={editorSaving}
          serverErrors={editorErrors}
        />
      )}
    </div>
  )
}
