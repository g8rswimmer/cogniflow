import { request, API_BASE, ApiError } from '../api/client'
import type {
  Workflow,
  WorkflowListResponse,
  NodeTypesResponse,
  Run,
  RunListResponse,
  TriggerRunResponse,
  EvalSuite,
  EvalSuiteListResponse,
  TestCase,
  TestCaseListResponse,
  EvalRun,
  EvalRunListResponse,
  TriggerEvalRunResponse,
  EvalRunCompare,
  ImportTestCasesResponse,
  GraderRegistration,
  GraderPluginsResponse,
} from '../api/types'

export const api = {
  listWorkflows: () =>
    request<WorkflowListResponse>('/workflows'),

  getWorkflow: (id: string) =>
    request<Workflow>(`/workflows/${id}`),

  createWorkflow: (body: unknown) =>
    request<Workflow>('/workflows', {
      method: 'POST',
      body: JSON.stringify(body),
    }),

  updateWorkflow: (id: string, body: unknown) =>
    request<Workflow>(`/workflows/${id}`, {
      method: 'PUT',
      body: JSON.stringify(body),
    }),

  deleteWorkflow: (id: string) =>
    request<void>(`/workflows/${id}`, { method: 'DELETE' }),

  listNodeTypes: () =>
    request<NodeTypesResponse>('/node-types'),

  triggerRun: (workflowId: string, initialData?: Record<string, unknown>) =>
    request<TriggerRunResponse>(`/workflows/${workflowId}/runs`, {
      method: 'POST',
      body: JSON.stringify({ initial_data: initialData ?? {} }),
    }),

  getRun: (runId: string) =>
    request<Run>(`/runs/${runId}`),

  listRuns: (workflowId: string) =>
    request<RunListResponse>(`/workflows/${workflowId}/runs`),

  // Eval Suite endpoints
  listEvalSuites: (workflowId: string) =>
    request<EvalSuiteListResponse>(`/workflows/${workflowId}/eval-suites`),

  createEvalSuite: (workflowId: string, body: unknown) =>
    request<EvalSuite>(`/workflows/${workflowId}/eval-suites`, {
      method: 'POST',
      body: JSON.stringify(body),
    }),

  getEvalSuite: (suiteId: string) =>
    request<EvalSuite>(`/eval-suites/${suiteId}`),

  updateEvalSuite: (suiteId: string, body: unknown) =>
    request<EvalSuite>(`/eval-suites/${suiteId}`, {
      method: 'PUT',
      body: JSON.stringify(body),
    }),

  deleteEvalSuite: (suiteId: string) =>
    request<void>(`/eval-suites/${suiteId}`, { method: 'DELETE' }),

  // Test Case endpoints
  listTestCases: (suiteId: string) =>
    request<TestCaseListResponse>(`/eval-suites/${suiteId}/test-cases`),

  createTestCase: (suiteId: string, body: unknown) =>
    request<TestCase>(`/eval-suites/${suiteId}/test-cases`, {
      method: 'POST',
      body: JSON.stringify(body),
    }),

  getTestCase: (suiteId: string, caseId: string) =>
    request<TestCase>(`/eval-suites/${suiteId}/test-cases/${caseId}`),

  updateTestCase: (suiteId: string, caseId: string, body: unknown) =>
    request<TestCase>(`/eval-suites/${suiteId}/test-cases/${caseId}`, {
      method: 'PUT',
      body: JSON.stringify(body),
    }),

  deleteTestCase: (suiteId: string, caseId: string) =>
    request<void>(`/eval-suites/${suiteId}/test-cases/${caseId}`, { method: 'DELETE' }),

  reorderTestCases: (suiteId: string, caseIds: string[]) =>
    request<void>(`/eval-suites/${suiteId}/test-cases/order`, {
      method: 'PUT',
      body: JSON.stringify({ case_ids: caseIds }),
    }),

  // Eval Run endpoints
  triggerEvalRun: (suiteId: string) =>
    request<TriggerEvalRunResponse>(`/eval-suites/${suiteId}/runs`, {
      method: 'POST',
      body: JSON.stringify({}),
    }),

  listEvalRuns: (suiteId: string, options?: { limit?: number }) => {
    const params = options?.limit ? `?limit=${options.limit}` : ''
    return request<EvalRunListResponse>(`/eval-suites/${suiteId}/runs${params}`)
  },

  getEvalRun: (runId: string) =>
    request<EvalRun>(`/eval-runs/${runId}`),

  compareEvalRuns: (headRunId: string, baselineRunId: string) =>
    request<EvalRunCompare>(
      `/eval-runs/${headRunId}/compare?baseline_run_id=${encodeURIComponent(baselineRunId)}`
    ),

  // Grader plugin admin
  listGraderPlugins: () =>
    request<GraderPluginsResponse>('/admin/grader-plugins'),

  registerGraderPlugin: (address: string) =>
    request<GraderRegistration>('/admin/grader-plugins', {
      method: 'POST',
      body: JSON.stringify({ address }),
    }),

  updateGraderPlugin: (typeId: string, address: string) =>
    request<GraderRegistration>(`/admin/grader-plugins/${encodeURIComponent(typeId)}`, {
      method: 'PUT',
      body: JSON.stringify({ address }),
    }),

  deleteGraderPlugin: (typeId: string) =>
    request<void>(`/admin/grader-plugins/${encodeURIComponent(typeId)}`, { method: 'DELETE' }),

  importTestCases: async (suiteId: string, file: File): Promise<ImportTestCasesResponse> => {
    const formData = new FormData()
    formData.append('file', file)
    // Do NOT set Content-Type — browser sets multipart/form-data with boundary automatically.
    const res = await fetch(`${API_BASE}/v1/eval-suites/${suiteId}/test-cases/import`, {
      method: 'POST',
      body: formData,
    })
    if (!res.ok) {
      const body = await res.json().catch(() => ({}))
      throw new ApiError(res.status, body)
    }
    return res.json() as Promise<ImportTestCasesResponse>
  },
}
