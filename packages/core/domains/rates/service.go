package rates

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/shared/clock"
)

// Fresh is how old a table may get before the service fetches a new one.
// Providers publish once a day.
const Fresh = 12 * time.Hour

// Repository stores the latest table. Latest returns nil when there is none.
type Repository interface {
	Latest(ctx context.Context) (*Table, error)
	Save(ctx context.Context, t Table) error
}

// Source fetches current rates from a provider.
type Source interface {
	Fetch(ctx context.Context) (Table, error)
}

// Service serves cached rates and refreshes them from the configured source.
type Service struct {
	repo   Repository
	source Source
	clock  clock.Clock
	log    *slog.Logger
	mu     sync.Mutex
}

// NewService builds a Service. A nil source turns rates off.
func NewService(repo Repository, source Source, c clock.Clock, log *slog.Logger) *Service {
	return &Service{repo: repo, source: source, clock: c, log: log}
}

// Enabled reports whether the host configured a rate source.
func (s *Service) Enabled() bool { return s.source != nil }

// Latest returns the newest table, fetching one first when the stored table
// is missing or stale. When the provider is down a stale table is still
// returned; nil means no rates are available at all.
func (s *Service) Latest(ctx context.Context) (*Table, error) {
	if !s.Enabled() {
		return nil, nil
	}
	t, err := s.repo.Latest(ctx)
	if err != nil {
		return nil, err
	}
	if t != nil && s.clock.Now().Sub(t.Fetched) < Fresh {
		return t, nil
	}
	fresh, err := s.refresh(ctx)
	if err == nil {
		return fresh, nil
	}
	// The provider is down: a stale table is better than none, but the
	// operator should know the rates are getting old.
	s.log.ErrorContext(ctx, "rates.refresh.failed", "stale", t != nil, "error", err)
	return t, nil
}

// Refresh fetches new rates when the stored ones are stale. Background jobs
// call it so requests rarely wait for the provider.
func (s *Service) Refresh(ctx context.Context) error {
	if !s.Enabled() {
		return nil
	}
	t, err := s.repo.Latest(ctx)
	if err != nil {
		return err
	}
	if t != nil && s.clock.Now().Sub(t.Fetched) < Fresh {
		return nil
	}
	_, err = s.refresh(ctx)
	return err
}

func (s *Service) refresh(ctx context.Context) (*Table, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Another request may have refreshed while this one waited.
	if t, err := s.repo.Latest(ctx); err == nil && t != nil && s.clock.Now().Sub(t.Fetched) < Fresh {
		return t, nil
	}
	t, err := s.source.Fetch(ctx)
	if err != nil {
		return nil, err
	}
	t.Fetched = s.clock.Now()
	if t.Published.IsZero() {
		t.Published = t.Fetched
	}
	if err := s.repo.Save(ctx, t); err != nil {
		return nil, err
	}
	return &t, nil
}
