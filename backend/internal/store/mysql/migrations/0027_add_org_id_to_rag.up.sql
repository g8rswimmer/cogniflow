ALTER TABLE rag_documents
    ADD COLUMN org_id VARCHAR(36) NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001',
    ADD INDEX idx_rag_documents_org_id (org_id);
