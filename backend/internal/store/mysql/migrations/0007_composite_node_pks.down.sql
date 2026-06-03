ALTER TABLE node_configs
    DROP INDEX uq_nc_wf_node_key,
    DROP INDEX idx_nc_node_id,
    DROP COLUMN workflow_id,
    ADD UNIQUE KEY uq_nc_node_key (node_id, config_key),
    ADD INDEX idx_nc_node_id (node_id);

ALTER TABLE workflow_edges
    DROP PRIMARY KEY,
    ADD PRIMARY KEY (id),
    ADD INDEX idx_we_workflow_id (workflow_id);

ALTER TABLE workflow_nodes
    DROP PRIMARY KEY,
    ADD PRIMARY KEY (id),
    ADD INDEX idx_wn_workflow_id (workflow_id);
