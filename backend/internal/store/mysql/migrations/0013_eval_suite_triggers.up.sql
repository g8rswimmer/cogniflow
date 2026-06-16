-- Add trigger config to eval_suites so suites can be scheduled (cron) or
-- triggered from CI pipelines (webhook). trigger_config is a JSON blob that
-- holds kind-specific fields: {"cron_expr":"..."} for cron or
-- {"webhook_secret":"enc:..."} (AES-256-GCM encrypted) for webhook.
ALTER TABLE eval_suites
    ADD COLUMN trigger_kind   VARCHAR(20) NOT NULL DEFAULT 'none'          AFTER workflow_deleted,
    ADD COLUMN trigger_config JSON        NOT NULL DEFAULT (JSON_OBJECT()) AFTER trigger_kind;

-- Record how an eval run was triggered so UI and logs can distinguish
-- manual, scheduled, and CI-webhook runs.
ALTER TABLE eval_runs
    ADD COLUMN triggered_by VARCHAR(20) NOT NULL DEFAULT 'manual' AFTER suite_id;
