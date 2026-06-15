import { useEffect, useState } from 'react'
import { useParams, Link, useNavigate } from 'react-router-dom'
import { api } from '../hooks/useApi'
import { useEvalStore } from '../stores/useEvalStore'
import { EvalSuiteForm } from '../components/eval/EvalSuiteForm'
import type { EvalSuite } from '../api/types'

export function EvalSuiteListPage() {
  const { id: workflowId } = useParams<{ id: string }>()
  const navigate = useNavigate()

  const suites = useEvalStore(s => s.suites)
  const setSuites = useEvalStore(s => s.setSuites)
  const upsertSuite = useEvalStore(s => s.upsertSuite)
  const removeSuite = useEvalStore(s => s.removeSuite)

  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [showForm, setShowForm] = useState(false)
  const [editingSuite, setEditingSuite] = useState<EvalSuite | null>(null)
  const [formSaving, setFormSaving] = useState(false)
  const [formError, setFormError] = useState<string | undefined>()
  const [deleting, setDeleting] = useState<string | null>(null)

  useEffect(() => {
    if (!workflowId) return
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setLoading(true)
    api.listEvalSuites(workflowId)
      .then(r => setSuites(r.eval_suites ?? []))
      .catch(err => setLoadError(err instanceof Error ? err.message : 'Failed to load suites'))
      .finally(() => setLoading(false))
  }, [workflowId, setSuites])

  const handleCreate = async (data: {
    name: string
    description?: string
    pass_threshold: number
    max_concurrency: number
  }) => {
    if (!workflowId) return
    setFormSaving(true)
    setFormError(undefined)
    try {
      const suite = await api.createEvalSuite(workflowId, data)
      upsertSuite(suite)
      setShowForm(false)
    } catch (err) {
      setFormError(err instanceof Error ? err.message : 'Failed to create suite')
    } finally {
      setFormSaving(false)
    }
  }

  const handleUpdate = async (data: {
    name: string
    description?: string
    pass_threshold: number
    max_concurrency: number
  }) => {
    if (!editingSuite) return
    setFormSaving(true)
    setFormError(undefined)
    try {
      const suite = await api.updateEvalSuite(editingSuite.id, data)
      upsertSuite(suite)
      setEditingSuite(null)
    } catch (err) {
      setFormError(err instanceof Error ? err.message : 'Failed to update suite')
    } finally {
      setFormSaving(false)
    }
  }

  const handleDelete = async (suite: EvalSuite) => {
    if (!confirm(`Delete suite "${suite.name}"? This cannot be undone.`)) return
    setDeleting(suite.id)
    try {
      await api.deleteEvalSuite(suite.id)
      removeSuite(suite.id)
    } catch (err) {
      alert(err instanceof Error ? err.message : 'Delete failed')
    } finally {
      setDeleting(null)
    }
  }

  const closeForm = () => {
    setShowForm(false)
    setEditingSuite(null)
    setFormError(undefined)
  }

  return (
    <div className="min-h-screen bg-gray-950 text-gray-100">
      <div className="max-w-4xl mx-auto px-4 py-8">
        {/* Header */}
        <div className="flex items-center justify-between mb-6">
          <div className="flex items-center gap-4">
            <Link
              to={`/workflows/${workflowId}`}
              className="text-indigo-400 hover:text-indigo-300 text-sm transition-colors"
            >
              ← Back to Editor
            </Link>
            <h1 className="text-xl font-bold text-gray-100">Eval Suites</h1>
          </div>
          <button
            onClick={() => setShowForm(true)}
            className="rounded-md bg-indigo-600 hover:bg-indigo-500 text-white text-xs font-semibold px-3 py-1.5 transition-colors"
          >
            + New Suite
          </button>
        </div>

        {loading && <p className="text-gray-400 text-sm">Loading suites…</p>}
        {loadError && <p className="text-red-400 text-sm">{loadError}</p>}

        {!loading && !loadError && suites.length === 0 && (
          <div className="text-center py-16">
            <p className="text-gray-500 text-sm mb-2">No eval suites yet.</p>
            <p className="text-gray-600 text-xs">
              Create a suite to start automated quality testing for this workflow.
            </p>
          </div>
        )}

        <div className="space-y-3">
          {suites.map(suite => (
            <div
              key={suite.id}
              className="rounded-lg bg-gray-800 border border-gray-700 px-4 py-3 hover:bg-gray-700 transition-colors group cursor-pointer"
              onClick={() => navigate(`/eval-suites/${suite.id}`)}
            >
              <div className="flex items-start justify-between gap-4">
                <div className="min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-medium text-gray-100">{suite.name}</span>
                    {suite.workflow_deleted && (
                      <span className="text-xs px-1.5 py-0.5 rounded bg-amber-900/60 text-amber-300">
                        workflow deleted
                      </span>
                    )}
                  </div>
                  {suite.description && (
                    <p className="text-xs text-gray-500 mt-0.5 truncate">{suite.description}</p>
                  )}
                  <div className="flex items-center gap-3 mt-1">
                    <span className="text-xs text-gray-500">
                      Pass threshold: {Math.round(suite.pass_threshold * 100)}%
                    </span>
                    <span className="text-xs text-gray-600">
                      Created {new Date(suite.created_at).toLocaleDateString()}
                    </span>
                  </div>
                </div>
                <div
                  className="flex gap-2 opacity-0 group-hover:opacity-100 transition-opacity"
                  onClick={e => e.stopPropagation()}
                >
                  <button
                    type="button"
                    onClick={() => { setEditingSuite(suite); setFormError(undefined) }}
                    className="text-xs text-indigo-400 hover:text-indigo-300 transition-colors px-2 py-1 rounded hover:bg-gray-700"
                  >
                    Edit
                  </button>
                  <button
                    type="button"
                    onClick={() => handleDelete(suite)}
                    disabled={deleting === suite.id}
                    className="text-xs text-red-500 hover:text-red-400 transition-colors px-2 py-1 rounded hover:bg-gray-700 disabled:opacity-50"
                  >
                    {deleting === suite.id ? '…' : 'Delete'}
                  </button>
                </div>
              </div>
            </div>
          ))}
        </div>
      </div>

      {showForm && (
        <EvalSuiteForm
          onSave={handleCreate}
          onClose={closeForm}
          saving={formSaving}
          error={formError}
        />
      )}

      {editingSuite && (
        <EvalSuiteForm
          suite={editingSuite}
          onSave={handleUpdate}
          onClose={closeForm}
          saving={formSaving}
          error={formError}
        />
      )}
    </div>
  )
}
