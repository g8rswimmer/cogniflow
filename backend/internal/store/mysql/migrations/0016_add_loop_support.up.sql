ALTER TABLE workflow_edges
    ADD COLUMN is_loop_back TINYINT(1) NOT NULL DEFAULT 0;
