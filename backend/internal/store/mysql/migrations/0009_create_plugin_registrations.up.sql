CREATE TABLE plugin_registrations (
    type_id       VARCHAR(100)  NOT NULL,
    address       VARCHAR(500)  NOT NULL,
    display_name  VARCHAR(255)  NOT NULL,
    category      VARCHAR(100)  NOT NULL DEFAULT 'plugin',
    description   TEXT,
    input_schema  JSON          NOT NULL,
    output_schema JSON          NOT NULL,
    registered_at DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (type_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
