export interface Position {
  x: number
  y: number
}

// ---------------------------------------------------------------------------
// Conditional node — structured rule format
// ---------------------------------------------------------------------------

export type ConditionalOperator = '==' | '!=' | '>' | '>=' | '<' | '<=' | 'contains'

export interface ConditionalCondition {
  node_id: string
  field: string
  operator: ConditionalOperator
  value: string
  value_type: 'string' | 'number' | 'boolean'
}

export interface ConditionalRule {
  label: string
  logic: 'AND' | 'OR'
  conditions: ConditionalCondition[]
}

// ExitCondition is structurally identical to ConditionalCondition; aliased for clarity.
export type ExitCondition = ConditionalCondition

// ---------------------------------------------------------------------------

export type OutputParserKind = 'json_path' | 'regex'

export interface OutputParser {
  kind: OutputParserKind
  source: string
  pattern: string
  capture_group?: number
}

export interface WorkflowNode {
  id: string
  type_id: string
  label: string
  position: Position
  config: Record<string, unknown>
  output_parsers?: Record<string, OutputParser>
}

export interface WorkflowEdge {
  id: string
  source_id: string
  target_id: string
  branch_label?: string | null
  is_loop_back?: boolean
}

export type TriggerKind = 'manual' | 'webhook' | 'cron'

export interface Trigger {
  kind: TriggerKind
  cron_expr?: string
  webhook_url?: string
}

export interface Workflow {
  id: string
  name: string
  description?: string
  trigger: Trigger
  timeout_seconds: number
  nodes: WorkflowNode[]
  edges: WorkflowEdge[]
  initial_data_schema?: Record<string, unknown> | null
  created_at: string
  updated_at: string
}

export interface NodeMeta {
  type_id: string
  display_name: string
  category: string
  description: string
  input_schema: Record<string, unknown>
  output_schema: Record<string, unknown>
}

export interface NodeTypesResponse {
  node_types: NodeMeta[]
}

export interface WorkflowSummary {
  id: string
  name: string
  description?: string
  trigger_kind: string
  timeout_seconds: number
  node_count: number
  created_at: string
  updated_at: string
}

export interface WorkflowListResponse {
  workflows: WorkflowSummary[]
}

export interface FieldValidationError {
  node_id?: string
  field?: string
  message: string
}

export interface ApiErrorBody {
  error?: {
    code?: string
    message?: string
    details?: {
      validation_errors?: FieldValidationError[]
    }
  }
}

export type NodeEventType =
  | 'node.pending'
  | 'node.running'
  | 'node.succeeded'
  | 'node.failed'
  | 'run.succeeded'
  | 'run.failed'

export interface NodeEvent {
  run_id: string
  node_id: string
  type: NodeEventType
  timestamp: string
  output?: Record<string, unknown>
  error?: string
}

export type RunStatus = 'pending' | 'running' | 'succeeded' | 'failed'

export interface NodeResult {
  status: 'succeeded' | 'failed'
  output?: Record<string, unknown>
  error?: string
}

export interface Run {
  run_id: string
  workflow_id: string
  status: RunStatus
  triggered_by: 'manual' | 'webhook' | 'cron'
  started_at: string
  finished_at?: string
  final_output?: Record<string, Record<string, unknown>>
  error_detail?: unknown
  node_results?: Record<string, NodeResult>
}

export interface RunListResponse {
  runs: Run[]
}

export interface TriggerRunResponse {
  run_id: string
  workflow_id: string
  status: RunStatus
  triggered_by: string
  started_at: string
}

// ---------------------------------------------------------------------------
// Eval types
// ---------------------------------------------------------------------------

export type EvalTriggerKind = 'none' | 'cron' | 'webhook'

export type GraderType = 'string_match' | 'numeric_threshold' | 'llm_judge' | 'json_schema' | 'checklist'
export type GraderScope = 'workflow' | 'node'
export type EvalRunStatus = 'pending' | 'running' | 'completed' | 'failed'
export type VerdictType = 'pass' | 'fail' | 'error'

export interface GraderDef {
  id: string
  name: string
  type: GraderType
  scope: GraderScope
  node_id?: string
  config: Record<string, unknown>
}

export interface NodeMock {
  node_id: string
  output: Record<string, unknown>
}

export interface TestCase {
  id: string
  suite_id: string
  name: string
  description?: string
  position: number
  initial_data: Record<string, unknown>
  mocks: NodeMock[]
  graders: GraderDef[]
  created_at: string
  updated_at: string
}

export interface EvalSuite {
  id: string
  workflow_id: string
  name: string
  description?: string
  pass_threshold: number
  max_concurrency?: number
  workflow_deleted?: boolean
  trigger_kind?: EvalTriggerKind
  cron_expr?: string
  webhook_url?: string
  webhook_secret?: string
  created_at: string
  updated_at: string
}

export interface EvalSuiteListResponse {
  eval_suites: EvalSuite[]
}

export interface TestCaseListResponse {
  test_cases: TestCase[]
}

export interface CriterionResult {
  criterion: string
  met: boolean
  explanation: string
}

export interface GraderResult {
  grader_id: string
  grader_name: string
  grader_type: GraderType
  verdict: VerdictType
  explanation?: string
  actual_value?: unknown
  score?: number
  criteria_results?: CriterionResult[]
}

export interface TestCaseResult {
  id: string
  eval_run_id: string
  test_case_id: string
  test_case_name: string
  workflow_run_id: string
  workflow_run_status: RunStatus
  passed: boolean
  grader_results: GraderResult[]
  node_outputs?: Record<string, Record<string, unknown>>
  created_at: string
}

export interface EvalRun {
  id: string
  suite_id: string
  status: EvalRunStatus
  triggered_by?: 'manual' | 'cron' | 'webhook'
  total_cases: number
  passed_count: number
  failed_count: number
  error_count: number
  started_at?: string
  finished_at?: string
  created_at: string
  test_case_results?: TestCaseResult[]
}

export interface EvalRunListResponse {
  eval_runs: EvalRun[]
}

export interface TriggerEvalRunResponse {
  id: string
  suite_id: string
  status: EvalRunStatus
  total_cases: number
  created_at: string
}

export type CompareChangeType = 'regressed' | 'improved' | 'unchanged' | 'new_case' | 'missing'

export interface TestCaseComparison {
  test_case_id: string
  test_case_name: string
  change_type: CompareChangeType
  head_passed: boolean | null
  baseline_passed: boolean | null
  head_result_id?: string
  baseline_result_id?: string
}

export interface EvalRunCompare {
  head_run_id: string
  baseline_run_id: string
  suite_id: string
  regressed_count: number
  improved_count: number
  unchanged_count: number
  new_case_count: number
  missing_count: number
  cases: TestCaseComparison[]
}

export interface ImportTestCasesResponse {
  created: number
  skipped: number
  errors: Array<{ row: number; message: string }>
}
