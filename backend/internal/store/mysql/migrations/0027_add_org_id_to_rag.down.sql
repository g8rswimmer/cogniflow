ALTER TABLE rag_documents
    DROP INDEX idx_rag_documents_org_id,
    DROP COLUMN org_id;
