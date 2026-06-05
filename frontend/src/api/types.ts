export interface Position {
  x: number
  y: number
}

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

export interface WorkflowListResponse {
  workflows: Workflow[]
}

export interface ApiErrorBody {
  error?: {
    code?: string
    message?: string
  }
}
