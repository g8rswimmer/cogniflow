CREATE TABLE grader_registrations (
    type_id       VARCHAR(100)  NOT NULL PRIMARY KEY,
    address       VARCHAR(500)  NOT NULL,
    display_name  VARCHAR(255)  NOT NULL,
    description   TEXT,
    config_schema JSON          NOT NULL,
    registered_at DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
