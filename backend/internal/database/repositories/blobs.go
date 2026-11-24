package repositories

import (
	"bytes"
	"context"
	"errors"
	"io"

	"github.com/atb00ker/splitspends/backend/internal/core/shared/errs"
	"github.com/atb00ker/splitspends/backend/internal/database"
	"github.com/jackc/pgx/v5"
)

// Blobs implements ports.BlobStore inside PostgreSQL, for hosts without a
// persistent disk or object store (serverless free tiers).
type Blobs struct{ db *database.DB }

// NewBlobs builds Blobs.
func NewBlobs(db *database.DB) *Blobs { return &Blobs{db: db} }

func (r *Blobs) Put(ctx context.Context, key string, rd io.Reader) error {
	data, err := io.ReadAll(rd)
	if err != nil {
		return err
	}
	_, err = r.db.Q(ctx).Exec(ctx, `INSERT INTO blobs (key, data) VALUES ($1, $2) ON CONFLICT (key) DO UPDATE SET data = EXCLUDED.data`, key, data)
	return err
}

func (r *Blobs) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	var data []byte
	err := r.db.Q(ctx).QueryRow(ctx, `SELECT data FROM blobs WHERE key = $1`, key).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errs.NotFound("File")
	}
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (r *Blobs) Delete(ctx context.Context, key string) error {
	_, err := r.db.Q(ctx).Exec(ctx, `DELETE FROM blobs WHERE key = $1`, key)
	return err
}
