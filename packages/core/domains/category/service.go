package category

import (
	"context"
	"fmt"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/expense"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/clock"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/google/uuid"
)

// Service implements category use cases.
type Service struct {
	repo  Repository
	clock clock.Clock
}

// NewService builds a Service.
func NewService(repo Repository, c clock.Clock) *Service {
	return &Service{repo: repo, clock: c}
}

// List returns the built-in categories followed by the custom categories the
// actor may see: their own and those used by expenses they can see.
func (s *Service) List(ctx context.Context, actor uuid.UUID) ([]*Category, error) {
	custom, err := s.repo.Visible(ctx, actor)
	if err != nil {
		return nil, err
	}
	builtIn := expense.Categories()
	out := make([]*Category, 0, len(builtIn)+len(custom))
	for _, c := range builtIn {
		out = append(out, &Category{ID: c.ID, Name: c.Name, Icon: c.Icon, Group: c.Group})
	}
	return append(out, custom...), nil
}

// owned returns the actor's custom category; other people's and deleted ones
// are not found.
func (s *Service) owned(ctx context.Context, actor uuid.UUID, id string) (*Category, error) {
	if !expense.CustomCategory(id) {
		return nil, errs.NotFound("Category")
	}
	c, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if c.OwnerID != actor || c.DeletedAt != nil {
		return nil, errs.NotFound("Category")
	}
	return c, nil
}

// Create adds a custom category. Repeating a create with the same id returns
// the existing category.
func (s *Service) Create(ctx context.Context, actor uuid.UUID, in Input) (*Category, error) {
	now := s.clock.Now()
	c, err := New(actor, in, now)
	if err != nil {
		return nil, err
	}
	if in.ID != nil && *in.ID != "" {
		existing, err := s.repo.Get(ctx, c.ID)
		if err == nil {
			if existing.OwnerID != actor {
				return nil, errs.Conflict("That category id is already in use.")
			}
			return existing, nil
		}
		if !errs.Is(err, errs.KindNotFound) {
			return nil, err
		}
	}
	mine, err := s.repo.Owned(ctx, actor)
	if err != nil {
		return nil, err
	}
	if len(mine) >= MaxPerUser {
		return nil, errs.Conflict(fmt.Sprintf("You can keep up to %d categories of your own.", MaxPerUser))
	}
	if err := s.repo.Create(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// Update renames a custom category or changes its icon or group.
func (s *Service) Update(ctx context.Context, actor uuid.UUID, id string, in Input) (*Category, error) {
	c, err := s.owned(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	in.ID = nil
	if err := c.Apply(in, s.clock.Now()); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// Delete hides a custom category. Expenses keep its id and show as General.
func (s *Service) Delete(ctx context.Context, actor uuid.UUID, id string) error {
	c, err := s.owned(ctx, actor, id)
	if err != nil {
		return err
	}
	now := s.clock.Now()
	c.DeletedAt, c.UpdatedAt = &now, now
	return s.repo.Update(ctx, c)
}

// Usable implements expense.Catalog: people file expenses under their own
// live categories.
func (s *Service) Usable(ctx context.Context, userID uuid.UUID, id string) (bool, error) {
	_, err := s.owned(ctx, userID, id)
	if errs.Is(err, errs.KindNotFound) {
		return false, nil
	}
	return err == nil, err
}

// Names returns the display name of every live category among ids, built-in
// or custom. Ids that are missing were deleted or never existed.
func (s *Service) Names(ctx context.Context, ids []string) (map[string]string, error) {
	out := map[string]string{}
	var custom []string
	for _, id := range ids {
		if c, ok := expense.BuiltIn(id); ok {
			out[id] = c.Name
		} else if expense.CustomCategory(id) {
			custom = append(custom, id)
		}
	}
	if len(custom) == 0 {
		return out, nil
	}
	live, err := s.repo.Live(ctx, custom)
	if err != nil {
		return nil, err
	}
	for _, c := range live {
		out[c.ID] = c.Name
	}
	return out, nil
}
