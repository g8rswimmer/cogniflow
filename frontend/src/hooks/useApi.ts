import { request, publicRequest, API_BASE, ApiError } from '../api/client'
import type {
  Workflow,
  WorkflowListResponse,
  WorkflowVersionListResponse,
  WorkflowVersionResponse,
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
  AuthResponse,
  UserResponse,
  InvitePreviewResponse,
  InviteCreatedResponse,
  OrgUsersResponse,
  OrgsResponse,
  AllUsersResponse,
  CreateOrgResponse,
  OrgEmailSettingsResponse,
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

  // Workflow Version endpoints
  listWorkflowVersions: (workflowId: string) =>
    request<WorkflowVersionListResponse>(`/workflows/${workflowId}/versions`),

  getWorkflowVersion: (workflowId: string, versionNumber: number) =>
    request<WorkflowVersionResponse>(`/workflows/${workflowId}/versions/${versionNumber}`),

  restoreWorkflowVersion: (workflowId: string, versionNumber: number) =>
    request<Workflow>(`/workflows/${workflowId}/versions/${versionNumber}/restore`, {
      method: 'POST',
    }),

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

  // ---- Auth (public — no token required) ------------------------------------

  login: (email: string, password: string) =>
    publicRequest<AuthResponse>('/auth/login', {
      method: 'POST',
      body: JSON.stringify({ email, password }),
    }),

  getInvite: (token: string) =>
    publicRequest<InvitePreviewResponse>(`/auth/invite/${encodeURIComponent(token)}`),

  acceptInvite: (token: string, password: string) =>
    publicRequest<AuthResponse>('/auth/accept-invite', {
      method: 'POST',
      body: JSON.stringify({ token, password }),
    }),

  // ---- Auth (authenticated) -------------------------------------------------

  getMe: () =>
    request<UserResponse>('/auth/me'),

  // ---- Org-admin ------------------------------------------------------------

  listOrgUsers: () =>
    request<OrgUsersResponse>('/org/users'),

  inviteUser: (body: { email: string; role: string; permissions?: string[] }) =>
    request<InviteCreatedResponse>('/org/users/invite', {
      method: 'POST',
      body: JSON.stringify(body),
    }),

  updateOrgUserRole: (userId: string, role: string) =>
    request<void>(`/org/users/${userId}/role`, {
      method: 'PUT',
      body: JSON.stringify({ role }),
    }),

  updateOrgUserPermissions: (userId: string, permissions: string[]) =>
    request<void>(`/org/users/${userId}/permissions`, {
      method: 'PUT',
      body: JSON.stringify({ permissions }),
    }),

  removeOrgUser: (userId: string) =>
    request<void>(`/org/users/${userId}`, { method: 'DELETE' }),

  // ---- System-admin ---------------------------------------------------------

  listOrgs: () =>
    request<OrgsResponse>('/admin/orgs'),

  createOrg: (body: { name: string; admin_email: string; admin_password: string }) =>
    request<CreateOrgResponse>('/admin/orgs', {
      method: 'POST',
      body: JSON.stringify(body),
    }),

  deleteOrg: (orgId: string) =>
    request<void>(`/admin/orgs/${orgId}`, { method: 'DELETE' }),

  listAllUsers: () =>
    request<AllUsersResponse>('/admin/users'),

  deleteUser: (userId: string) =>
    request<void>(`/admin/users/${userId}`, { method: 'DELETE' }),

  adminUpdateUserRole: (userId: string, role: string) =>
    request<void>(`/admin/users/${userId}/role`, {
      method: 'PUT',
      body: JSON.stringify({ role }),
    }),

  adminUpdateUserPermissions: (userId: string, permissions: string[]) =>
    request<void>(`/admin/users/${userId}/permissions`, {
      method: 'PUT',
      body: JSON.stringify({ permissions }),
    }),

  getOrgEmailSettings: () =>
    request<OrgEmailSettingsResponse>('/org/email-settings'),

  setOrgEmailSettingsAdmin: (orgId: string, body: {
    smtp_host: string; smtp_port: string; smtp_user: string
    smtp_password: string; smtp_from: string; subject: string; body: string
  }) =>
    request<OrgEmailSettingsResponse>(`/admin/orgs/${encodeURIComponent(orgId)}/email-settings`, {
      method: 'PUT',
      body: JSON.stringify(body),
    }),

  upsertOrgEmailSettings: (body: {
    smtp_host: string; smtp_port: string; smtp_user: string
    smtp_password: string; smtp_from: string; subject: string; body: string
  }) =>
    request<OrgEmailSettingsResponse>('/org/email-settings', {
      method: 'PUT',
      body: JSON.stringify(body),
    }),

  deleteOrgEmailSettings: () =>
    request<void>('/org/email-settings', { method: 'DELETE' }),
}
