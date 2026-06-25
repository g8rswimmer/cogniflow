ALTER TABLE eval_suites
    DROP INDEX idx_eval_suites_org_id,
    DROP COLUMN org_id;
