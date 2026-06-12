-- eval_suites
-- No FOREIGN KEY to workflows — referential integrity at app layer.
CREATE TABLE eval_suites (
    id               VARCHAR(36)       NOT NULL,
    workflow_id      VARCHAR(36)       NOT NULL,
    name             VARCHAR(255)      NOT NULL,
    description      TEXT,
    pass_threshold   DECIMAL(4,3)      NOT NULL DEFAULT 1.000,
    max_concurrency  TINYINT UNSIGNED  NOT NULL DEFAULT 1,
    workflow_deleted TINYINT(1)        NOT NULL DEFAULT 0,
    created_at       DATETIME(3)       NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at       DATETIME(3)       NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
                                       ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    INDEX idx_es_workflow_id (workflow_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- eval_test_cases
-- mocks and graders are JSON blobs; sensitive api_key values within graders
-- are AES-256-GCM encrypted (same cipher as node_configs.encrypted_value).
CREATE TABLE eval_test_cases (
    id           VARCHAR(36)   NOT NULL,
    suite_id     VARCHAR(36)   NOT NULL,
    name         VARCHAR(255)  NOT NULL,
    description  TEXT,
    position     INT UNSIGNED  NOT NULL DEFAULT 0,
    initial_data JSON          NOT NULL,
    mocks        JSON          NOT NULL DEFAULT (JSON_ARRAY()),
    graders      JSON          NOT NULL DEFAULT (JSON_ARRAY()),
    created_at   DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at   DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
                               ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    INDEX idx_etc_suite_position (suite_id, position)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- eval_runs
-- passed_count / failed_count / error_count updated atomically during execution.
CREATE TABLE eval_runs (
    id           VARCHAR(36)  NOT NULL,
    suite_id     VARCHAR(36)  NOT NULL,
    status       VARCHAR(20)  NOT NULL DEFAULT 'pending',
    total_cases  INT UNSIGNED NOT NULL DEFAULT 0,
    passed_count INT UNSIGNED NOT NULL DEFAULT 0,
    failed_count INT UNSIGNED NOT NULL DEFAULT 0,
    error_count  INT UNSIGNED NOT NULL DEFAULT 0,
    started_at   DATETIME(3),
    finished_at  DATETIME(3),
    created_at   DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    INDEX idx_er_suite_id (suite_id),
    INDEX idx_er_suite_status (suite_id, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- eval_test_case_results
-- node_outputs stores only outputs for nodes referenced by scope:"node" graders.
CREATE TABLE eval_test_case_results (
    id                  VARCHAR(36)  NOT NULL,
    eval_run_id         VARCHAR(36)  NOT NULL,
    test_case_id        VARCHAR(36)  NOT NULL,
    test_case_name      VARCHAR(255) NOT NULL,
    workflow_run_id     VARCHAR(36)  NOT NULL,
    workflow_run_status VARCHAR(20)  NOT NULL,
    node_outputs        JSON         NOT NULL DEFAULT (JSON_OBJECT()),
    grader_results      JSON         NOT NULL DEFAULT (JSON_ARRAY()),
    passed              TINYINT(1)   NOT NULL DEFAULT 0,
    created_at          DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    INDEX idx_etcr_eval_run_id (eval_run_id),
    INDEX idx_etcr_workflow_run_id (workflow_run_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
