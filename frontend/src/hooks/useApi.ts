import { request } from '../api/client'
import type {
  Workflow,
  WorkflowListResponse,
  NodeTypesResponse,
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
}
