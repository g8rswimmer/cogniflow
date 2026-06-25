CREATE TABLE org_email_settings (
    org_id        VARCHAR(36)   NOT NULL PRIMARY KEY,
    smtp_host     VARCHAR(255)  NOT NULL DEFAULT '',
    smtp_port     VARCHAR(10)   NOT NULL DEFAULT '587',
    smtp_user     VARCHAR(255)  NOT NULL DEFAULT '',
    smtp_password VARCHAR(2048) NOT NULL DEFAULT '',
    smtp_from     VARCHAR(255)  NOT NULL DEFAULT '',
    subject       TEXT          NOT NULL,
    body          TEXT          NOT NULL,
    updated_at    DATETIME      NOT NULL
);
