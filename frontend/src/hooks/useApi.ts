import { request } from '../api/client'
import type {
  Workflow,
  WorkflowListResponse,
  NodeTypesResponse,
  Run,
  RunListResponse,
  TriggerRunResponse,
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
}
