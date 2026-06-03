package mysql

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
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
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO rag_chunks (id, document_id, chunk_index, chunk_text, embedding)
			 VALUES (?, ?, ?, ?, ?)`,
			c.ID, c.DocumentID, c.ChunkIndex, c.ChunkText, embeddingToBytes(c.Embedding),
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
// using cosine distance computed in Go. Results are ordered ascending (lower = more similar).
// When docFilter is non-empty only chunks for that document_id are searched.
func (s *WorkflowStore) SearchChunks(ctx context.Context, embedding []float32, topK int, docFilter string) ([]store.RAGChunkResult, error) {
	if topK <= 0 {
		topK = 5
	}

	q := `SELECT id, chunk_text, embedding FROM rag_chunks`
	var args []any
	if docFilter != "" {
		q += " WHERE document_id = ?"
		args = append(args, docFilter)
	}

	type dbRow struct {
		ID        string `db:"id"`
		ChunkText string `db:"chunk_text"`
		Embedding []byte `db:"embedding"`
	}
	var rows []dbRow
	if err := s.db.SelectContext(ctx, &rows, q, args...); err != nil {
		return nil, fmt.Errorf("rag store: search chunks: %w", err)
	}

	type candidate struct {
		id        string
		chunkText string
		score     float32
	}
	candidates := make([]candidate, 0, len(rows))
	for _, r := range rows {
		emb := bytesToEmbedding(r.Embedding)
		if len(emb) != len(embedding) {
			continue // skip dimension mismatch
		}
		candidates = append(candidates, candidate{
			id:        r.ID,
			chunkText: r.ChunkText,
			score:     cosineDist(embedding, emb),
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score < candidates[j].score
	})

	if topK > len(candidates) {
		topK = len(candidates)
	}
	results := make([]store.RAGChunkResult, topK)
	for i := range results {
		results[i] = store.RAGChunkResult{
			ID:        candidates[i].id,
			ChunkText: candidates[i].chunkText,
			Score:     candidates[i].score,
		}
	}
	return results, nil
}

// embeddingToBytes encodes a []float32 as little-endian raw bytes for MEDIUMBLOB storage.
func embeddingToBytes(v []float32) []byte {
	b := make([]byte, 4*len(v))
	for i, f := range v {
		binary.LittleEndian.PutUint32(b[4*i:], math.Float32bits(f))
	}
	return b
}

// bytesToEmbedding decodes little-endian raw bytes back to a []float32.
func bytesToEmbedding(b []byte) []float32 {
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[4*i:]))
	}
	return v
}

// cosineDist returns the cosine distance (1 - cosine similarity) between two vectors.
// Returns 1.0 for zero-length vectors.
func cosineDist(a, b []float32) float32 {
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 1.0
	}
	return float32(1.0 - dot/(math.Sqrt(normA)*math.Sqrt(normB)))
}
