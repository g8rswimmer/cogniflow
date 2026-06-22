CREATE TABLE workflow_versions (
    id             VARCHAR(36)       NOT NULL,
    workflow_id    VARCHAR(36)       NOT NULL,
    version_number INT UNSIGNED      NOT NULL,
    node_count     SMALLINT UNSIGNED NOT NULL DEFAULT 0,
    definition     LONGTEXT          NOT NULL,
    created_at     DATETIME(3)       NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    KEY idx_wv_workflow_version (workflow_id, version_number)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
