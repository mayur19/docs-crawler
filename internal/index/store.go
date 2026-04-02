package index

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"

	"github.com/mayur19/docs-crawler/internal/pipeline"
	_ "modernc.org/sqlite"
)

const driverName = "sqlite"

const schemaSQL = `
CREATE TABLE IF NOT EXISTS chunks (
	id TEXT PRIMARY KEY,
	source_url TEXT NOT NULL,
	title TEXT NOT NULL,
	heading_path TEXT NOT NULL,
	content TEXT NOT NULL,
	token_count INTEGER NOT NULL,
	content_hash TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS embeddings (
	chunk_id TEXT PRIMARY KEY REFERENCES chunks(id),
	vector BLOB NOT NULL,
	dimensions INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS meta (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
CREATE VIRTUAL TABLE IF NOT EXISTS chunks_fts USING fts5(content, title, heading_path);
`

// SQLiteStore is a persistent store backed by SQLite.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore opens or creates a SQLite DB at dbPath and applies the schema.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	db, err := sql.Open(driverName, dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}

	if _, err := db.Exec(schemaSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

// Name returns the store identifier.
func (s *SQLiteStore) Name() string {
	return "sqlite"
}

// Close closes the underlying database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// Index upserts a batch of EmbeddedChunks and their FTS5 entries within a transaction.
func (s *SQLiteStore) Index(ctx context.Context, chunks []pipeline.EmbeddedChunk) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	for _, ec := range chunks {
		if err := upsertChunk(ctx, tx, ec.Chunk); err != nil {
			return fmt.Errorf("upsert chunk %s: %w", ec.Chunk.ID, err)
		}
		if err := upsertEmbedding(ctx, tx, ec); err != nil {
			return fmt.Errorf("upsert embedding %s: %w", ec.Chunk.ID, err)
		}
		if err := upsertFTS(ctx, tx, ec.Chunk); err != nil {
			return fmt.Errorf("upsert fts %s: %w", ec.Chunk.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// GetAllChunks returns every chunk stored in the database.
func (s *SQLiteStore) GetAllChunks(ctx context.Context) ([]pipeline.Chunk, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, source_url, title, heading_path, content, token_count, content_hash
		FROM chunks
	`)
	if err != nil {
		return nil, fmt.Errorf("query chunks: %w", err)
	}
	defer rows.Close()

	var result []pipeline.Chunk
	for rows.Next() {
		chunk, err := scanChunk(rows)
		if err != nil {
			return nil, fmt.Errorf("scan chunk: %w", err)
		}
		result = append(result, chunk)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return result, nil
}

// GetEmbedding retrieves the vector embedding for a specific chunk ID.
func (s *SQLiteStore) GetEmbedding(ctx context.Context, chunkID string) ([]float32, error) {
	var blob []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT vector FROM embeddings WHERE chunk_id = ?
	`, chunkID).Scan(&blob)
	if err != nil {
		return nil, fmt.Errorf("get embedding for chunk %s: %w", chunkID, err)
	}
	return bytesToVector(blob), nil
}

// SetMeta stores a key-value metadata pair (upsert).
func (s *SQLiteStore) SetMeta(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT OR REPLACE INTO meta (key, value) VALUES (?, ?)
	`, key, value)
	if err != nil {
		return fmt.Errorf("set meta key %s: %w", key, err)
	}
	return nil
}

// GetMeta retrieves a metadata value by key.
func (s *SQLiteStore) GetMeta(ctx context.Context, key string) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = ?`, key).Scan(&value)
	if err != nil {
		return "", fmt.Errorf("get meta key %s: %w", key, err)
	}
	return value, nil
}

// DeleteBySourceURL removes all chunks (and their embeddings/FTS entries) for a given URL.
func (s *SQLiteStore) DeleteBySourceURL(ctx context.Context, sourceURL string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Delete FTS rows by rowid matching the chunk IDs
	_, err = tx.ExecContext(ctx, `
		DELETE FROM chunks_fts WHERE rowid IN (
			SELECT rowid FROM chunks WHERE source_url = ?
		)
	`, sourceURL)
	if err != nil {
		return fmt.Errorf("delete fts for source %s: %w", sourceURL, err)
	}

	// Delete embeddings for those chunks
	_, err = tx.ExecContext(ctx, `
		DELETE FROM embeddings WHERE chunk_id IN (
			SELECT id FROM chunks WHERE source_url = ?
		)
	`, sourceURL)
	if err != nil {
		return fmt.Errorf("delete embeddings for source %s: %w", sourceURL, err)
	}

	// Delete chunks
	_, err = tx.ExecContext(ctx, `DELETE FROM chunks WHERE source_url = ?`, sourceURL)
	if err != nil {
		return fmt.Errorf("delete chunks for source %s: %w", sourceURL, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// --- internal helpers ---

type scanner interface {
	Scan(dest ...any) error
}

func upsertChunk(ctx context.Context, tx *sql.Tx, chunk pipeline.Chunk) error {
	headingJSON, err := json.Marshal(chunk.HeadingPath)
	if err != nil {
		return fmt.Errorf("marshal heading path: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT OR REPLACE INTO chunks
			(id, source_url, title, heading_path, content, token_count, content_hash)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, chunk.ID, chunk.SourceURL, chunk.Title, string(headingJSON),
		chunk.Content, chunk.TokenCount, chunk.ContentHash)
	return err
}

func upsertEmbedding(ctx context.Context, tx *sql.Tx, ec pipeline.EmbeddedChunk) error {
	blob := vectorToBytes(ec.Vector)
	_, err := tx.ExecContext(ctx, `
		INSERT OR REPLACE INTO embeddings (chunk_id, vector, dimensions)
		VALUES (?, ?, ?)
	`, ec.Chunk.ID, blob, ec.Dimensions)
	return err
}

func upsertFTS(ctx context.Context, tx *sql.Tx, chunk pipeline.Chunk) error {
	headingJSON, err := json.Marshal(chunk.HeadingPath)
	if err != nil {
		return fmt.Errorf("marshal heading path for fts: %w", err)
	}

	// Delete existing FTS entry for this chunk's rowid first to handle upsert
	_, err = tx.ExecContext(ctx, `
		DELETE FROM chunks_fts WHERE rowid = (SELECT rowid FROM chunks WHERE id = ?)
	`, chunk.ID)
	if err != nil {
		return fmt.Errorf("delete existing fts entry: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT OR REPLACE INTO chunks_fts (rowid, content, title, heading_path)
		VALUES ((SELECT rowid FROM chunks WHERE id = ?), ?, ?, ?)
	`, chunk.ID, chunk.Content, chunk.Title, string(headingJSON))
	return err
}

func scanChunk(row scanner) (pipeline.Chunk, error) {
	var (
		id, sourceURL, title, headingJSON, content, contentHash string
		tokenCount                                               int
	)
	if err := row.Scan(&id, &sourceURL, &title, &headingJSON, &content, &tokenCount, &contentHash); err != nil {
		return pipeline.Chunk{}, err
	}

	var headingPath []string
	if err := json.Unmarshal([]byte(headingJSON), &headingPath); err != nil {
		return pipeline.Chunk{}, fmt.Errorf("unmarshal heading path: %w", err)
	}

	return pipeline.Chunk{
		ID:          id,
		SourceURL:   sourceURL,
		Title:       title,
		HeadingPath: headingPath,
		Content:     content,
		TokenCount:  tokenCount,
		ContentHash: contentHash,
	}, nil
}

// vectorToBytes encodes a float32 slice as a little-endian byte slice.
func vectorToBytes(vec []float32) []byte {
	buf := make([]byte, len(vec)*4)
	for i, v := range vec {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	return buf
}

// bytesToVector decodes a little-endian byte slice into a float32 slice.
func bytesToVector(data []byte) []float32 {
	n := len(data) / 4
	vec := make([]float32, n)
	for i := range vec {
		bits := binary.LittleEndian.Uint32(data[i*4:])
		vec[i] = math.Float32frombits(bits)
	}
	return vec
}
