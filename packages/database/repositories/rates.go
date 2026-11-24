package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/rates"
	"github.com/Hostworthy/SplitSpends/packages/database"
	"github.com/jackc/pgx/v5"
)

// Rates stores the latest exchange-rate table.
type Rates struct{ db *database.DB }

// NewRates builds Rates.
func NewRates(db *database.DB) *Rates { return &Rates{db: db} }

func (r *Rates) Latest(ctx context.Context) (*rates.Table, error) {
	var t rates.Table
	var raw []byte
	err := r.db.Q(ctx).QueryRow(ctx, `SELECT base, published_at, fetched_at, source, rates FROM exchange_rates ORDER BY fetched_at DESC LIMIT 1`).
		Scan(&t.Base, &t.Published, &t.Fetched, &t.Source, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var values map[string]string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, err
	}
	t.Rates = make(map[string]*big.Rat, len(values))
	for code, v := range values {
		if q, ok := new(big.Rat).SetString(v); ok {
			t.Rates[code] = q
		}
	}
	return &t, nil
}

func (r *Rates) Save(ctx context.Context, t rates.Table) error {
	values := make(map[string]string, len(t.Rates))
	for code, v := range t.Rates {
		values[code] = rates.Decimal(v)
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return err
	}
	// One table is kept: a new base replaces the old one.
	return database.NewTx(r.db).Do(ctx, func(ctx context.Context) error {
		if _, err := r.db.Q(ctx).Exec(ctx, `DELETE FROM exchange_rates WHERE base <> $1`, t.Base); err != nil {
			return err
		}
		_, err := r.db.Q(ctx).Exec(ctx, `INSERT INTO exchange_rates (base, published_at, fetched_at, source, rates) VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (base) DO UPDATE SET published_at = EXCLUDED.published_at, fetched_at = EXCLUDED.fetched_at, source = EXCLUDED.source, rates = EXCLUDED.rates`,
			t.Base, t.Published, t.Fetched, t.Source, raw)
		return err
	})
}
