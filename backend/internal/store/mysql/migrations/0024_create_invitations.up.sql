CREATE TABLE IF NOT EXISTS invitations (
    id          VARCHAR(36)                   NOT NULL PRIMARY KEY,
    org_id      VARCHAR(36)                   NOT NULL,
    email       VARCHAR(255)                  NOT NULL,
    role        ENUM('org_admin','member')    NOT NULL DEFAULT 'member',
    permissions JSON                          NOT NULL,
    token       VARCHAR(64)                   NOT NULL,
    invited_by  VARCHAR(36)                   NOT NULL,
    expires_at  DATETIME(3)                   NOT NULL,
    accepted_at DATETIME(3)                   NULL,
    created_at  DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    UNIQUE KEY uq_invitations_token (token),
    INDEX idx_invitations_org_id (org_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
