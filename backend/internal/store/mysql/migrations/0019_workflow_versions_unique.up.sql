ALTER TABLE workflow_versions
    ADD UNIQUE KEY uq_wv_workflow_version (workflow_id, version_number);
