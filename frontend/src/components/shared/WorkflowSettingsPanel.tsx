import { useRef, useEffect } from 'react'
import { useWorkflowStore } from '../../stores/useWorkflowStore'
import type { Trigger, TriggerKind } from '../../api/types'
import { InitialDataSchemaEditor } from '../sidebar/InitialDataSchemaEditor'

interface Props {
  workflowId: string | null
  onClose: () => void
}

export function WorkflowSettingsPanel({ workflowId, onClose }: Props) {
  const name = useWorkflowStore(s => s.name)
  const setName = useWorkflowStore(s => s.setName)
  const description = useWorkflowStore(s => s.description)
  const setDescription = useWorkflowStore(s => s.setDescription)
  const trigger = useWorkflowStore(s => s.trigger)
  const setTrigger = useWorkflowStore(s => s.setTrigger)
  const initialDataSchema = useWorkflowStore(s => s.initialDataSchema)

  const fieldCount = Object.keys(
    (initialDataSchema?.properties as Record<string, unknown> | undefined) ?? {}
  ).length

  const webhookUrl = workflowId ? `/webhooks/${workflowId}` : '(save workflow first)'

  // Refs survive kind switches within the session so config isn't lost when
  // the user temporarily selects a different kind and comes back.
  const lastCronExpr = useRef(trigger.cron_expr ?? '* * * * *')
  const lastKafkaBrokers = useRef(trigger.kafka_brokers ?? '')
  const lastKafkaTopic = useRef(trigger.kafka_topic ?? '')
  const lastKafkaGroupID = useRef(trigger.kafka_group_id ?? '')
  const lastSQSQueueURL = useRef(trigger.sqs_queue_url ?? '')
  const lastSQSRegion = useRef(trigger.sqs_region ?? '')

  const handleKindChange = (kind: TriggerKind) => {
    const next: Trigger = { kind }
    if (kind === 'cron') next.cron_expr = lastCronExpr.current
    if (kind === 'webhook' && workflowId) next.webhook_url = webhookUrl
    if (kind === 'kafka') {
      next.kafka_brokers = lastKafkaBrokers.current
      next.kafka_topic = lastKafkaTopic.current
      if (lastKafkaGroupID.current) next.kafka_group_id = lastKafkaGroupID.current
    }
    if (kind === 'sqs') {
      next.sqs_queue_url = lastSQSQueueURL.current
      next.sqs_region = lastSQSRegion.current
    }
    setTrigger(next)
  }

  const handleCronChange = (cron_expr: string) => {
    lastCronExpr.current = cron_expr
    setTrigger({ kind: 'cron', cron_expr })
  }

  useEffect(() => {
    const handler = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose() }
    document.addEventListener('keydown', handler)
    return () => document.removeEventListener('keydown', handler)
  }, [onClose])

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60"
      onClick={onClose}
    >
      <div
        className="bg-gray-800 border border-gray-700 rounded-xl shadow-2xl w-[32rem] flex flex-col max-h-[90vh]"
        onClick={e => e.stopPropagation()}
      >
        {/* Header */}
        <div className="flex items-center justify-between px-5 pt-5 pb-4 flex-shrink-0">
          <h2 className="text-base font-semibold text-gray-100">⚙ Workflow Settings</h2>
          <button
            onClick={onClose}
            className="text-gray-500 hover:text-gray-300 transition-colors"
          >
            ✕
          </button>
        </div>

        {/* Scrollable body */}
        <div className="overflow-y-auto px-5 flex-1">

          {/* General */}
          <div className="mb-5">
            <div className="text-[10px] font-semibold uppercase tracking-widest text-gray-500 mb-3">
              General
            </div>
            <div className="space-y-3">
              <div>
                <label className="text-xs text-gray-400 block mb-1">Name</label>
                <input
                  type="text"
                  value={name}
                  onChange={e => setName(e.target.value)}
                  placeholder="Workflow name"
                  className="
                    w-full rounded-md bg-gray-700 border border-gray-600
                    px-3 py-2 text-sm text-gray-100
                    focus:outline-none focus:border-indigo-500
                  "
                />
              </div>
              <div>
                <label className="text-xs text-gray-400 block mb-1">Description</label>
                <textarea
                  value={description}
                  onChange={e => setDescription(e.target.value)}
                  placeholder="Optional description of what this workflow does"
                  rows={3}
                  className="
                    w-full rounded-md bg-gray-700 border border-gray-600
                    px-3 py-2 text-sm text-gray-100 resize-none
                    focus:outline-none focus:border-indigo-500
                  "
                />
              </div>
            </div>
          </div>

          <div className="border-t border-gray-700 mb-5" />

          {/* Trigger */}
          <div className="mb-5">
            <div className="text-[10px] font-semibold uppercase tracking-widest text-gray-500 mb-3">
              Trigger
            </div>

            {/* Kind selector */}
            <div className="flex flex-wrap gap-2 mb-4">
              {(['manual', 'webhook', 'cron', 'kafka', 'sqs'] as TriggerKind[]).map(k => (
                <button
                  key={k}
                  onClick={() => handleKindChange(k)}
                  className={`
                    px-3 py-2 rounded-lg text-sm font-medium capitalize transition-colors border
                    ${trigger.kind === k
                      ? 'bg-indigo-600 border-indigo-500 text-white'
                      : 'bg-gray-700 border-gray-600 text-gray-300 hover:bg-gray-600'}
                  `}
                >
                  {k}
                </button>
              ))}
            </div>

            {trigger.kind === 'manual' && (
              <div className="text-sm text-gray-400">
                Workflow runs only when triggered via the "Run" button or the API.
              </div>
            )}

            {trigger.kind === 'webhook' && (
              <div className="rounded-md bg-gray-700/60 border border-gray-600 p-3">
                <div className="text-xs text-gray-400 mb-1">Inbound webhook URL</div>
                <div className="font-mono text-sm text-indigo-300 break-all">{webhookUrl}</div>
                <div className="text-xs text-gray-500 mt-2">
                  POST JSON to this URL to trigger the workflow. The body becomes initial data.
                </div>
              </div>
            )}

            {trigger.kind === 'cron' && (
              <div>
                <label className="text-xs text-gray-400 block mb-1">
                  Cron expression (5-field, UTC)
                </label>
                <input
                  type="text"
                  value={trigger.cron_expr ?? ''}
                  onChange={e => handleCronChange(e.target.value)}
                  placeholder="* * * * *"
                  className="
                    w-full rounded-md bg-gray-700 border border-gray-600
                    px-3 py-2 font-mono text-sm text-gray-100
                    focus:outline-none focus:border-indigo-500
                  "
                />
                <div className="text-xs text-gray-500 mt-1">
                  min hour day month weekday — e.g.{' '}
                  <code className="text-gray-400">0 9 * * 1-5</code>
                </div>
              </div>
            )}

            {trigger.kind === 'kafka' && (
              <div className="space-y-3">
                <div>
                  <label className="text-xs text-gray-400 block mb-1">Brokers</label>
                  <input
                    type="text"
                    value={trigger.kafka_brokers ?? ''}
                    onChange={e => {
                      lastKafkaBrokers.current = e.target.value
                      setTrigger({ ...trigger, kafka_brokers: e.target.value })
                    }}
                    placeholder="localhost:9092"
                    className="
                      w-full rounded-md bg-gray-700 border border-gray-600
                      px-3 py-2 font-mono text-sm text-gray-100
                      focus:outline-none focus:border-indigo-500
                    "
                  />
                  <div className="text-xs text-gray-500 mt-1">Comma-separated host:port list</div>
                </div>
                <div>
                  <label className="text-xs text-gray-400 block mb-1">Topic</label>
                  <input
                    type="text"
                    value={trigger.kafka_topic ?? ''}
                    onChange={e => {
                      lastKafkaTopic.current = e.target.value
                      setTrigger({ ...trigger, kafka_topic: e.target.value })
                    }}
                    placeholder="my-topic"
                    className="
                      w-full rounded-md bg-gray-700 border border-gray-600
                      px-3 py-2 font-mono text-sm text-gray-100
                      focus:outline-none focus:border-indigo-500
                    "
                  />
                </div>
                <div>
                  <label className="text-xs text-gray-400 block mb-1">
                    Consumer Group ID{' '}
                    <span className="text-gray-600 normal-case font-normal">(optional)</span>
                  </label>
                  <input
                    type="text"
                    value={trigger.kafka_group_id ?? ''}
                    onChange={e => {
                      lastKafkaGroupID.current = e.target.value
                      setTrigger({ ...trigger, kafka_group_id: e.target.value })
                    }}
                    placeholder={workflowId ? `cogniflow-${workflowId}` : 'cogniflow-…'}
                    className="
                      w-full rounded-md bg-gray-700 border border-gray-600
                      px-3 py-2 font-mono text-sm text-gray-100
                      focus:outline-none focus:border-indigo-500
                    "
                  />
                </div>
                <div className="text-xs text-gray-500">
                  Each message triggers one run. The message body (JSON) becomes the run's initial data.
                </div>
              </div>
            )}

            {trigger.kind === 'sqs' && (
              <div className="space-y-3">
                <div>
                  <label className="text-xs text-gray-400 block mb-1">Queue URL</label>
                  <input
                    type="text"
                    value={trigger.sqs_queue_url ?? ''}
                    onChange={e => {
                      lastSQSQueueURL.current = e.target.value
                      setTrigger({ ...trigger, sqs_queue_url: e.target.value })
                    }}
                    placeholder="https://sqs.us-east-1.amazonaws.com/123456789/my-queue"
                    className="
                      w-full rounded-md bg-gray-700 border border-gray-600
                      px-3 py-2 font-mono text-sm text-gray-100
                      focus:outline-none focus:border-indigo-500
                    "
                  />
                </div>
                <div>
                  <label className="text-xs text-gray-400 block mb-1">Region</label>
                  <input
                    type="text"
                    value={trigger.sqs_region ?? ''}
                    onChange={e => {
                      lastSQSRegion.current = e.target.value
                      setTrigger({ ...trigger, sqs_region: e.target.value })
                    }}
                    placeholder="us-east-1"
                    className="
                      w-full rounded-md bg-gray-700 border border-gray-600
                      px-3 py-2 font-mono text-sm text-gray-100
                      focus:outline-none focus:border-indigo-500
                    "
                  />
                </div>
                <div className="text-xs text-gray-500">
                  Uses the standard AWS credential chain (env vars, IAM role). Messages are deleted after successful dispatch.
                </div>
              </div>
            )}
          </div>

          <div className="border-t border-gray-700 mb-5" />

          {/* Inputs */}
          <div className="mb-5">
            <div className="text-[10px] font-semibold uppercase tracking-widest text-gray-500 mb-3">
              Inputs
            </div>
            {fieldCount > 0 && (
              <p className="text-xs text-gray-500 mb-4">
                {fieldCount} field{fieldCount !== 1 ? 's' : ''} declared — referenced in nodes as{' '}
                <code className="text-indigo-300 bg-gray-900 px-1 rounded">
                  {'{{._initial.field}}'}
                </code>.
              </p>
            )}
            <InitialDataSchemaEditor hideHeading />
          </div>
        </div>

        {/* Footer */}
        <div className="flex justify-end px-5 py-4 border-t border-gray-700 flex-shrink-0">
          <button
            onClick={onClose}
            className="
              rounded-md bg-indigo-600 hover:bg-indigo-500
              px-4 py-1.5 text-xs text-white font-semibold
              transition-colors
            "
          >
            Close
          </button>
        </div>
      </div>
    </div>
  )
}
