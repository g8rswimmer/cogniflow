ALTER TABLE workflows
    DROP INDEX idx_workflows_org_id,
    DROP COLUMN org_id;
