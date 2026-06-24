-- Seed the default org so existing rows get a valid org_id.
INSERT IGNORE INTO organizations (id, name, created_at)
VALUES ('00000000-0000-0000-0000-000000000001', 'Default', NOW());

ALTER TABLE workflows
    ADD COLUMN org_id VARCHAR(36) NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001',
    ADD INDEX idx_workflows_org_id (org_id);
