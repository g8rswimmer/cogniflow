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

export interface Run {
  run_id: string
  workflow_id: string
  status: RunStatus
  triggered_by: 'manual' | 'webhook' | 'cron'
  started_at: string
  finished_at?: string
  final_output?: Record<string, Record<string, unknown>>
  error_detail?: unknown
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
