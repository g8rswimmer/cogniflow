package mysql

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/g8rswimmer/cogniflow/internal/store"
)

// UpsertChunks ingests a batch of chunks for one or more documents.
// For each document_id in the batch the existing chunks are deleted first,
// then the new chunks are inserted. This ensures that re-ingesting a document
// with fewer chunks does not leave stale rows from the previous version.
// The document record in rag_documents is created if it does not yet exist.
// All changes are made inside a single transaction.
func (s *WorkflowStore) UpsertChunks(ctx context.Context, chunks []store.RAGChunk) error {
	if len(chunks) == 0 {
		return nil
	}

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("rag store: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	now := time.Now().UTC()

	// Collect the unique document IDs in this batch.
	docIDs := make(map[string]struct{})
	for _, c := range chunks {
		docIDs[c.DocumentID] = struct{}{}
	}

	for docID := range docIDs {
		// Ensure the document record exists (no-op if already present).
		if _, err := tx.ExecContext(ctx,
			`INSERT IGNORE INTO rag_documents (id, source, created_at) VALUES (?, ?, ?)`,
			docID, docID, now,
		); err != nil {
			return fmt.Errorf("rag store: upsert document %q: %w", docID, err)
		}
		// Delete all existing chunks for this document so that re-ingestion
		// with a shorter text does not leave stale chunks that would pollute
		// future retrieval results.
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM rag_chunks WHERE document_id = ?`, docID,
		); err != nil {
			return fmt.Errorf("rag store: clear stale chunks for %q: %w", docID, err)
		}
	}

	for _, c := range chunks {
		vecJSON, err := json.Marshal(c.Embedding)
		if err != nil {
			return fmt.Errorf("rag store: marshal embedding for chunk %q: %w", c.ID, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO rag_chunks (id, document_id, chunk_index, chunk_text, embedding)
			 VALUES (?, ?, ?, ?, STRING_TO_VECTOR(?))`,
			c.ID, c.DocumentID, c.ChunkIndex, c.ChunkText, string(vecJSON),
		); err != nil {
			return fmt.Errorf("rag store: insert chunk %q: %w", c.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("rag store: commit: %w", err)
	}
	return nil
}

// SearchChunks retrieves the top-K most similar chunks to the given embedding
// using VEC_DISTANCE_COSINE. Results are ordered by score ascending (lower = more similar).
// When docFilter is non-empty only chunks for that document_id are searched.
//
// Note: MySQL 9.0's VECTOR INDEX (ANN) requires a pure ORDER BY distance LIMIT query
// with no additional WHERE predicates to engage the index. When docFilter is set the
// optimizer falls back to a full-table scan computing cosine distance for every matching
// row. This is acceptable for small corpora but degrades at large scale (100K+ chunks).
func (s *WorkflowStore) SearchChunks(ctx context.Context, embedding []float32, topK int, docFilter string) ([]store.RAGChunkResult, error) {
	if topK <= 0 {
		topK = 5
	}

	vecJSON, err := json.Marshal(embedding)
	if err != nil {
		return nil, fmt.Errorf("rag store: marshal query embedding: %w", err)
	}

	q := `SELECT id, chunk_text,
		         VEC_DISTANCE_COSINE(embedding, STRING_TO_VECTOR(?)) AS score
		  FROM rag_chunks`
	args := []any{string(vecJSON)}

	if docFilter != "" {
		q += " WHERE document_id = ?"
		args = append(args, docFilter)
	}
	q += " ORDER BY score ASC LIMIT ?"
	args = append(args, topK)

	type dbResult struct {
		ID        string  `db:"id"`
		ChunkText string  `db:"chunk_text"`
		Score     float32 `db:"score"`
	}

	var rows []dbResult
	if err := s.db.SelectContext(ctx, &rows, q, args...); err != nil {
		return nil, fmt.Errorf("rag store: search chunks: %w", err)
	}

	results := make([]store.RAGChunkResult, len(rows))
	for i, r := range rows {
		results[i] = store.RAGChunkResult{
			ID:        r.ID,
			ChunkText: r.ChunkText,
			Score:     r.Score,
		}
	}
	return results, nil
}
