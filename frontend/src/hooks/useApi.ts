import { request } from '../api/client'
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

  listEvalRuns: (suiteId: string) =>
    request<EvalRunListResponse>(`/eval-suites/${suiteId}/runs`),

  getEvalRun: (runId: string) =>
    request<EvalRun>(`/eval-runs/${runId}`),
}
