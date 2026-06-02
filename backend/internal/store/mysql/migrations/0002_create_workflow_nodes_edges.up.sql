CREATE TABLE workflow_nodes (
    id               VARCHAR(36)      NOT NULL,
    workflow_id      VARCHAR(36)      NOT NULL,
    type_id          VARCHAR(100)     NOT NULL,
    label            VARCHAR(255),
    position_x       DOUBLE           NOT NULL DEFAULT 0,
    position_y       DOUBLE           NOT NULL DEFAULT 0,
    retry_max        TINYINT UNSIGNED NOT NULL DEFAULT 0,
    retry_backoff_ms INT UNSIGNED     NOT NULL DEFAULT 1000,
    PRIMARY KEY (id),
    INDEX idx_wn_workflow_id (workflow_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE workflow_edges (
    id           VARCHAR(36)  NOT NULL,
    workflow_id  VARCHAR(36)  NOT NULL,
    source_id    VARCHAR(36)  NOT NULL,
    target_id    VARCHAR(36)  NOT NULL,
    branch_label VARCHAR(20),
    PRIMARY KEY (id),
    INDEX idx_we_workflow_id (workflow_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE node_configs (
    id              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    node_id         VARCHAR(36)     NOT NULL,
    config_key      VARCHAR(255)    NOT NULL,
    plain_value     MEDIUMTEXT,
    encrypted_value MEDIUMBLOB,
    is_sensitive    TINYINT(1)      NOT NULL DEFAULT 0,
    PRIMARY KEY (id),
    UNIQUE KEY uq_nc_node_key (node_id, config_key),
    INDEX idx_nc_node_id (node_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
