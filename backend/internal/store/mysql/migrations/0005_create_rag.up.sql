CREATE TABLE rag_documents (
    id         VARCHAR(255) NOT NULL,
    source     TEXT         NOT NULL,
    created_at DATETIME(6)  NOT NULL,
    PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE rag_chunks (
    id           VARCHAR(255) NOT NULL,
    document_id  VARCHAR(255) NOT NULL,
    chunk_index  INT          NOT NULL DEFAULT 0,
    chunk_text   MEDIUMTEXT   NOT NULL,
    embedding    VECTOR(1536) NOT NULL,
    PRIMARY KEY (id),
    INDEX idx_rag_chunks_doc (document_id),
    VECTOR INDEX idx_rag_chunks_emb (embedding)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
