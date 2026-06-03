-- Migration 0007 backfilled node_configs.workflow_id using an INNER JOIN.
-- Any node_configs rows whose node_id had no matching workflow_nodes entry
-- were left with workflow_id=''. Those rows are unreachable by all application
-- queries (which filter by workflow_id) and can never be cleaned up by normal
-- delete paths, so we remove them here.
DELETE FROM node_configs WHERE workflow_id = '';
