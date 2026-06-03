-- Make node and edge IDs workflow-scoped rather than globally unique.
-- Node IDs like "n1" only need to be unique within a workflow, not across
-- all workflows. Same applies to edge IDs.
--
-- workflow_nodes and workflow_edges: swap single-column PK for (workflow_id, id).
-- node_configs: add workflow_id so configs can be scoped and cleaned up correctly.

ALTER TABLE workflow_nodes
    DROP PRIMARY KEY,
    DROP INDEX idx_wn_workflow_id,
    ADD PRIMARY KEY (workflow_id, id);

ALTER TABLE workflow_edges
    DROP PRIMARY KEY,
    DROP INDEX idx_we_workflow_id,
    ADD PRIMARY KEY (workflow_id, id);

-- Add workflow_id to node_configs and backfill from workflow_nodes.
ALTER TABLE node_configs
    ADD COLUMN workflow_id VARCHAR(36) NOT NULL DEFAULT '';

UPDATE node_configs nc
    INNER JOIN workflow_nodes wn ON wn.id = nc.node_id
    SET nc.workflow_id = wn.workflow_id;

ALTER TABLE node_configs
    DROP INDEX uq_nc_node_key,
    DROP INDEX idx_nc_node_id,
    ADD UNIQUE KEY uq_nc_wf_node_key (workflow_id, node_id, config_key),
    ADD INDEX idx_nc_node_id (workflow_id, node_id);
