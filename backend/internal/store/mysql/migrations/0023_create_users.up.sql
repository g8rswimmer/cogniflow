CREATE TABLE IF NOT EXISTS users (
    id            VARCHAR(36)                              NOT NULL PRIMARY KEY,
    org_id        VARCHAR(36)                              NOT NULL,
    email         VARCHAR(255)                             NOT NULL,
    password_hash VARCHAR(255)                             NOT NULL,
    role          ENUM('system_admin','org_admin','member') NOT NULL DEFAULT 'member',
    permissions   JSON                                     NOT NULL,
    created_at    DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at    DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    UNIQUE KEY uq_users_email (email),
    INDEX idx_users_org_id (org_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
