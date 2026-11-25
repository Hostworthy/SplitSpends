package invite

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/group"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/user"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/google/uuid"
)

type memInvites struct{ list []*Invite }

func (m *memInvites) Create(_ context.Context, i *Invite) error {
	m.list = append(m.list, i)
	return nil
}
func (m *memInvites) FindByHash(_ context.Context, hash string) (*Invite, error) {
	for _, i := range m.list {
		if i.TokenHash == hash {
			return i, nil
		}
	}
	return nil, nil
}
func (m *memInvites) Active(_ context.Context, groupID uuid.UUID, now time.Time) ([]*Invite, error) {
	var out []*Invite
	for _, i := range m.list {
		if i.GroupID == groupID && !i.Expired(now) {
			out = append(out, i)
		}
	}
	return out, nil
}
func (m *memInvites) Delete(_ context.Context, groupID, id uuid.UUID) (bool, error) {
	for n, i := range m.list {
		if i.ID == id && i.GroupID == groupID {
			m.list = append(m.list[:n], m.list[n+1:]...)
			return true, nil
		}
	}
	return false, nil
}
func (m *memInvites) DeleteExpired(context.Context, time.Time) error { return nil }

// groups is a single group that implements Groups and group.Repository.
type groups struct{ g *group.Group }

func (f *groups) Member(_ context.Context, actor, id uuid.UUID) (*group.Group, error) {
	if id != f.g.ID || !f.g.HasMember(actor) {
		return nil, errs.NotFound("Group")
	}
	return f.g, nil
}
func (f *groups) Join(_ context.Context, actor, id uuid.UUID) (*group.View, error) {
	if id != f.g.ID {
		return nil, errs.NotFound("Group")
	}
	if !f.g.HasMember(actor) {
		f.g.Members = append(f.g.Members, actor)
	}
	return &group.View{Group: f.g}, nil
}
func (f *groups) Create(context.Context, *group.Group) error { return nil }
func (f *groups) Get(_ context.Context, id uuid.UUID) (*group.Group, error) {
	if id != f.g.ID {
		return nil, errs.NotFound("Group")
	}
	return f.g, nil
}
func (f *groups) Lock(ctx context.Context, id uuid.UUID) (*group.Group, error)        { return f.Get(ctx, id) }
func (f *groups) Update(context.Context, *group.Group) error                          { return nil }
func (f *groups) Delete(context.Context, uuid.UUID) error                             { return nil }
func (f *groups) ForUser(context.Context, uuid.UUID, bool) ([]*group.Group, error)    { return nil, nil }
func (f *groups) AddMember(context.Context, uuid.UUID, uuid.UUID, time.Time) error    { return nil }
func (f *groups) RemoveMember(context.Context, uuid.UUID, uuid.UUID, time.Time) error { return nil }

type users map[uuid.UUID]*user.User

func (m users) Get(_ context.Context, id uuid.UUID) (*user.User, error) {
	if u := m[id]; u != nil {
		return u, nil
	}
	return nil, errs.NotFound("User")
}
func (m users) FindByEmail(context.Context, string) (*user.User, error) { return nil, nil }
func (m users) List(context.Context, []uuid.UUID) ([]*user.User, error) { return nil, nil }
func (m users) Create(context.Context, *user.User) error                { return nil }
func (m users) Update(context.Context, *user.User) error                { return nil }
func (m users) Count(context.Context) (int, error)                      { return len(m), nil }
func (m users) Placeholders(context.Context, uuid.UUID) (int, error)    { return 0, nil }
func (m users) Lock(context.Context) error                              { return nil }

type clockAt struct{ at time.Time }

func (c *clockAt) Now() time.Time { return c.at }

// setup builds a service with Ada alone in her flat; others are further
// accounts that exist but are not members.
func setup(t *testing.T, others ...*user.User) (*Service, *groups, *user.User, *clockAt) {
	t.Helper()
	ada, err := user.New("ada@example.com", "Ada", user.StatusActive, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	accounts := users{ada.ID: ada}
	for _, u := range others {
		accounts[u.ID] = u
	}
	g := &groups{g: &group.Group{ID: uuid.New(), Name: "Flat", Emoji: "🏠", Members: []uuid.UUID{ada.ID}}}
	c := &clockAt{at: time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)}
	svc := NewService(&memInvites{}, g, g, user.NewService(accounts, nil, nil, c), c, "https://split.test")
	return svc, g, ada, c
}

func TestTokensAreRandomAndHashed(t *testing.T) {
	inv, token, err := New(uuid.New(), uuid.New(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != 32 {
		t.Fatalf("token must be 32 random bytes in base64url, got %q", token)
	}
	if inv.TokenHash == token || inv.TokenHash != Hash(token) || len(inv.TokenHash) != 64 {
		t.Fatalf("only the SHA-256 hash may be stored: %s", inv.TokenHash)
	}
	_, other, _ := New(uuid.New(), uuid.New(), time.Now())
	if other == token {
		t.Fatal("tokens must differ")
	}
}

func TestCreatePreviewAccept(t *testing.T) {
	ctx := context.Background()
	svc, g, ada, _ := setup(t)
	issued, err := svc.Create(ctx, ada.ID, g.g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if issued.URL != "https://split.test/join/"+issued.Token {
		t.Fatalf("unexpected url %s", issued.URL)
	}
	if issued.ExpiresAt.Sub(issued.CreatedAt) != 14*24*time.Hour {
		t.Fatalf("invites last 14 days, got %s", issued.ExpiresAt.Sub(issued.CreatedAt))
	}
	p, err := svc.Preview(ctx, issued.Token)
	if err != nil || p.Group.Name != "Flat" || p.Inviter.Name != "Ada" {
		t.Fatalf("preview: %+v %v", p, err)
	}
	ben := uuid.New()
	for i := 0; i < 2; i++ {
		v, err := svc.Accept(ctx, ben, issued.Token)
		if err != nil {
			t.Fatal(err)
		}
		if len(v.Members) != 2 {
			t.Fatalf("accepting twice must add the member once: %v", v.Members)
		}
	}
	// Multiple people can use the same link.
	if _, err := svc.Accept(ctx, uuid.New(), issued.Token); err != nil {
		t.Fatal(err)
	}
	list, err := svc.List(ctx, ada.ID, g.g.ID)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v %v", list, err)
	}
}

func TestOnlyMembersManageInvites(t *testing.T) {
	ctx := context.Background()
	svc, g, ada, _ := setup(t)
	stranger := uuid.New()
	if _, err := svc.Create(ctx, stranger, g.g.ID); !errs.Is(err, errs.KindNotFound) {
		t.Fatalf("strangers cannot create invites, got %v", err)
	}
	issued, _ := svc.Create(ctx, ada.ID, g.g.ID)
	if _, err := svc.List(ctx, stranger, g.g.ID); !errs.Is(err, errs.KindNotFound) {
		t.Fatalf("strangers cannot list invites, got %v", err)
	}
	if err := svc.Revoke(ctx, stranger, g.g.ID, issued.ID); !errs.Is(err, errs.KindNotFound) {
		t.Fatalf("strangers cannot revoke invites, got %v", err)
	}
	now := time.Now()
	g.g.ArchivedAt = &now
	if _, err := svc.Create(ctx, ada.ID, g.g.ID); !errs.Is(err, errs.KindConflict) {
		t.Fatalf("archived groups take no invites, got %v", err)
	}
}

func TestExpiredUnknownAndRevokedLookTheSame(t *testing.T) {
	ctx := context.Background()
	svc, g, ada, c := setup(t)
	expiring, _ := svc.Create(ctx, ada.ID, g.g.ID)
	revoked, _ := svc.Create(ctx, ada.ID, g.g.ID)
	if err := svc.Revoke(ctx, ada.ID, g.g.ID, revoked.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Revoke(ctx, ada.ID, g.g.ID, revoked.ID); !errs.Is(err, errs.KindNotFound) {
		t.Fatalf("revoking twice: %v", err)
	}
	c.at = c.at.Add(14 * 24 * time.Hour)
	tokens := []string{expiring.Token, revoked.Token, "unknown", "", strings.Repeat("a", 500)}
	var messages []string
	for _, token := range tokens {
		_, err := svc.Preview(ctx, token)
		if !errs.Is(err, errs.KindNotFound) {
			t.Fatalf("token %q: want not found, got %v", token, err)
		}
		messages = append(messages, err.Error())
		if ok, err := svc.Valid(ctx, token); ok || err != nil {
			t.Fatalf("token %q must not be valid: %v", token, err)
		}
		if _, err := svc.Accept(ctx, uuid.New(), token); !errs.Is(err, errs.KindNotFound) {
			t.Fatalf("token %q cannot be accepted: %v", token, err)
		}
	}
	for _, m := range messages {
		if m != messages[0] {
			t.Fatalf("errors must not reveal why a token fails: %v", messages)
		}
	}
	list, _ := svc.List(ctx, ada.ID, g.g.ID)
	if len(list) != 0 {
		t.Fatalf("expired invites are not active: %v", list)
	}
}

func TestLinksEndWhenTheirCreatorLeaves(t *testing.T) {
	ctx := context.Background()
	account, err := user.New("mal@example.com", "Mal", user.StatusActive, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	svc, g, ada, _ := setup(t, account)
	mal := account.ID
	g.g.Members = append(g.g.Members, mal)
	issued, err := svc.Create(ctx, mal, g.g.ID)
	if err != nil {
		t.Fatal(err)
	}
	kept, err := svc.Create(ctx, ada.ID, g.g.ID)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := svc.Lookup(ctx, issued.Token)
	if err != nil || hash == "" {
		t.Fatalf("the link works while Mal is a member: %q %v", hash, err)
	}
	g.g.Members = []uuid.UUID{ada.ID}

	if _, err := svc.Accept(ctx, mal, issued.Token); !errs.Is(err, errs.KindNotFound) {
		t.Fatalf("a link of someone who left must not work, got %v", err)
	}
	if g.g.HasMember(mal) {
		t.Fatal("Mal rejoined through his own link")
	}
	if _, err := svc.Preview(ctx, issued.Token); !errs.Is(err, errs.KindNotFound) {
		t.Fatalf("preview: %v", err)
	}
	if ok, err := svc.Live(ctx, hash); ok || err != nil {
		t.Fatalf("a link of someone who left must not let anyone sign up: %v", err)
	}
	list, err := svc.List(ctx, ada.ID, g.g.ID)
	if err != nil || len(list) != 1 || list[0].ID != kept.ID {
		t.Fatalf("only Ada's link is still usable: %v %v", list, err)
	}
	if _, err := svc.Accept(ctx, uuid.New(), kept.Token); err != nil {
		t.Fatalf("links of current members keep working: %v", err)
	}
}

func TestLinksEndWhenTheGroupIsArchived(t *testing.T) {
	ctx := context.Background()
	svc, g, ada, c := setup(t)
	issued, err := svc.Create(ctx, ada.ID, g.g.ID)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := svc.Lookup(ctx, issued.Token)
	if err != nil || hash == "" {
		t.Fatalf("the link works before archiving: %q %v", hash, err)
	}
	g.g.ArchivedAt = &c.at

	if _, err := svc.Preview(ctx, issued.Token); !errs.Is(err, errs.KindNotFound) {
		t.Fatalf("preview: %v", err)
	}
	if ok, err := svc.Valid(ctx, issued.Token); ok || err != nil {
		t.Fatalf("a link to an archived group must not be valid: %v", err)
	}
	if ok, err := svc.Live(ctx, hash); ok || err != nil {
		t.Fatalf("a link to an archived group must not let anyone sign up: %v", err)
	}
	ben := uuid.New()
	if _, err := svc.Accept(ctx, ben, issued.Token); !errs.Is(err, errs.KindNotFound) {
		t.Fatalf("a link to an archived group cannot be accepted: %v", err)
	}
	if g.g.HasMember(ben) {
		t.Fatal("Ben joined an archived group")
	}
}
