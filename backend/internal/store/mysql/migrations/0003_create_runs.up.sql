CREATE TABLE runs (
    id           VARCHAR(36)  NOT NULL,
    workflow_id  VARCHAR(36)  NOT NULL,
    triggered_by VARCHAR(20)  NOT NULL,
    status       VARCHAR(20)  NOT NULL DEFAULT 'pending',
    started_at   DATETIME(3),
    finished_at  DATETIME(3),
    final_output JSON,
    error_detail JSON,
    PRIMARY KEY (id),
    CONSTRAINT fk_r_workflow FOREIGN KEY (workflow_id)
        REFERENCES workflows (id) ON DELETE CASCADE,
    INDEX idx_runs_workflow_status_started (workflow_id, status, started_at),
    INDEX idx_runs_started_at (started_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
