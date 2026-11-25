package invite

import (
	"context"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/group"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/user"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/clock"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/google/uuid"
)

// Groups is what invites need from the group context.
type Groups interface {
	// Member returns the group if actor is an active member.
	Member(ctx context.Context, actor, id uuid.UUID) (*group.Group, error)
	// Join adds actor to the group; joining twice changes nothing.
	Join(ctx context.Context, actor, id uuid.UUID) (*group.View, error)
}

// Service creates, lists, revokes and redeems invite links.
type Service struct {
	repo    Repository
	groups  Groups
	lookup  group.Repository
	users   *user.Service
	clock   clock.Clock
	baseURL string
}

// NewService builds a Service. lookup reads groups without a membership
// check, for the public preview.
func NewService(repo Repository, groups Groups, lookup group.Repository, users *user.Service, c clock.Clock, baseURL string) *Service {
	return &Service{repo: repo, groups: groups, lookup: lookup, users: users, clock: c, baseURL: baseURL}
}

// Issued is a new invite with its token and link; both are shown only once.
type Issued struct {
	*Invite
	Token string
	URL   string
}

// Create makes an invite link for a group the actor belongs to.
func (s *Service) Create(ctx context.Context, actor, groupID uuid.UUID) (*Issued, error) {
	g, err := s.groups.Member(ctx, actor, groupID)
	if err != nil {
		return nil, err
	}
	if g.ArchivedAt != nil {
		return nil, errs.Conflict("Unarchive the group before inviting people.")
	}
	inv, token, err := New(g.ID, actor, s.clock.Now())
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, inv); err != nil {
		return nil, err
	}
	return &Issued{Invite: inv, Token: token, URL: s.baseURL + Path + token}, nil
}

// List returns a group's active invites, without tokens.
func (s *Service) List(ctx context.Context, actor, groupID uuid.UUID) ([]*Invite, error) {
	if _, err := s.groups.Member(ctx, actor, groupID); err != nil {
		return nil, err
	}
	return s.repo.Active(ctx, groupID, s.clock.Now())
}

// Revoke deletes an invite so its link stops working.
func (s *Service) Revoke(ctx context.Context, actor, groupID, id uuid.UUID) error {
	if _, err := s.groups.Member(ctx, actor, groupID); err != nil {
		return err
	}
	ok, err := s.repo.Delete(ctx, groupID, id)
	if err != nil {
		return err
	}
	if !ok {
		return errs.NotFound("Invite")
	}
	return nil
}

// find returns the usable invite for token. Unknown, revoked and expired
// tokens give the same error so links cannot be probed.
func (s *Service) find(ctx context.Context, token string) (*Invite, error) {
	missing := errs.NotFound("Invite")
	if token == "" || len(token) > 128 {
		return nil, missing
	}
	inv, err := s.repo.FindByHash(ctx, Hash(token))
	if err != nil {
		return nil, err
	}
	if inv == nil || inv.Expired(s.clock.Now()) {
		return nil, missing
	}
	return inv, nil
}

// Valid reports whether token is a usable invite.
func (s *Service) Valid(ctx context.Context, token string) (bool, error) {
	hash, err := s.Lookup(ctx, token)
	return hash != "", err
}

// Lookup returns the storage hash of token when it is a usable invite, or
// "" when it is not. Login uses it to let people sign up through a link, and
// keeps the hash to check the link again with Live.
func (s *Service) Lookup(ctx context.Context, token string) (string, error) {
	inv, err := s.find(ctx, token)
	if errs.Is(err, errs.KindNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return inv.TokenHash, nil
}

// Live reports whether the invite stored under hash is still usable.
func (s *Service) Live(ctx context.Context, hash string) (bool, error) {
	if hash == "" {
		return false, nil
	}
	inv, err := s.repo.FindByHash(ctx, hash)
	if err != nil || inv == nil {
		return false, err
	}
	return !inv.Expired(s.clock.Now()), nil
}

// Preview is the little that a link shows before joining.
type Preview struct {
	Group   *group.Group
	Inviter *user.User
}

// Preview describes the group behind a token, for anyone holding it.
func (s *Service) Preview(ctx context.Context, token string) (*Preview, error) {
	inv, err := s.find(ctx, token)
	if err != nil {
		return nil, err
	}
	g, err := s.lookup.Get(ctx, inv.GroupID)
	if err != nil {
		if errs.Is(err, errs.KindNotFound) {
			return nil, errs.NotFound("Invite")
		}
		return nil, err
	}
	inviter, err := s.users.Get(ctx, inv.CreatedBy)
	if err != nil {
		return nil, err
	}
	return &Preview{Group: g, Inviter: inviter}, nil
}

// Accept adds the actor to the invite's group. Accepting again, or as a
// member, simply returns the group.
func (s *Service) Accept(ctx context.Context, actor uuid.UUID, token string) (*group.View, error) {
	inv, err := s.find(ctx, token)
	if err != nil {
		return nil, err
	}
	return s.groups.Join(ctx, actor, inv.GroupID)
}

// Cleanup removes expired invites.
func (s *Service) Cleanup(ctx context.Context) error {
	return s.repo.DeleteExpired(ctx, s.clock.Now())
}
