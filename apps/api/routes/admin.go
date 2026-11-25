package routes

import (
	"context"
	"net/http"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/admin"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/user"
	"github.com/Hostworthy/SplitSpends/packages/platform"
	"github.com/google/uuid"
)

func toSignupSettings(s *admin.Settings) platform.SignupSettings {
	out := platform.SignupSettings{Mode: string(s.Mode), Allowed: s.Allowed}
	if out.Allowed == nil {
		out.Allowed = []string{}
	}
	if !s.UpdatedAt.IsZero() {
		at := s.UpdatedAt.UTC().Format(time.RFC3339)
		out.UpdatedAt = &at
	}
	return out
}

func toRequest(r *admin.Request) platform.SignupRequest {
	return platform.SignupRequest{ID: r.ID.String(), Email: r.Email, Name: r.Name, Status: string(r.Status),
		RequestedAt: stamp(r.RequestedAt), DecidedAt: stampPtr(r.DecidedAt)}
}

func toAccount(u *user.User) platform.Account {
	return platform.Account{User: toAccountUser(u), Role: string(u.Role), CreatedAt: stamp(u.CreatedAt)}
}

func (a *API) signupSettings(w http.ResponseWriter, r *http.Request) error {
	s, err := a.svc.Admin.Settings(r.Context(), me(r))
	if err != nil {
		return err
	}
	return details(w, http.StatusOK, toSignupSettings(s))
}

func (a *API) updateSignupSettings(w http.ResponseWriter, r *http.Request) error {
	in, err := decode[platform.SignupSettingsInput](r)
	if err != nil {
		return err
	}
	s, err := a.svc.Admin.UpdateSettings(r.Context(), me(r), in.Mode, in.Allowed)
	if err != nil {
		return err
	}
	return details(w, http.StatusOK, toSignupSettings(s))
}

func (a *API) signupRequests(w http.ResponseWriter, r *http.Request) error {
	reqs, err := a.svc.Admin.Requests(r.Context(), me(r), r.URL.Query().Get("status"))
	if err != nil {
		return err
	}
	out := make([]platform.SignupRequest, 0, len(reqs))
	for _, req := range reqs {
		out = append(out, toRequest(req))
	}
	return list(w, out, len(out))
}

func (a *API) approveRequest(w http.ResponseWriter, r *http.Request) error {
	return a.decideRequest(w, r, a.svc.Admin.Approve)
}

func (a *API) declineRequest(w http.ResponseWriter, r *http.Request) error {
	return a.decideRequest(w, r, a.svc.Admin.Decline)
}

func (a *API) decideRequest(w http.ResponseWriter, r *http.Request, act func(ctx context.Context, actor, id uuid.UUID) (*admin.Request, error)) error {
	id, err := pathID(r, "requestId", "Request")
	if err != nil {
		return err
	}
	req, err := act(r.Context(), me(r), id)
	if err != nil {
		return err
	}
	return details(w, http.StatusOK, toRequest(req))
}

func (a *API) accounts(w http.ResponseWriter, r *http.Request) error {
	limit, offset := page(r)
	q := r.URL.Query()
	users, total, err := a.svc.Admin.Accounts(r.Context(), me(r), q.Get("status"), q.Get("q"), limit, offset)
	if err != nil {
		return err
	}
	out := make([]platform.Account, 0, len(users))
	for _, u := range users {
		out = append(out, toAccount(u))
	}
	return list(w, out, total)
}

func (a *API) setRole(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "userId", "Account")
	if err != nil {
		return err
	}
	in, err := decode[platform.RoleInput](r)
	if err != nil {
		return err
	}
	u, err := a.svc.Admin.SetRole(r.Context(), me(r), id, in.Role)
	if err != nil {
		return err
	}
	return details(w, http.StatusOK, toAccount(u))
}

func (a *API) disableAccount(w http.ResponseWriter, r *http.Request) error {
	return a.toggleAccount(w, r, a.svc.Admin.Disable)
}

func (a *API) enableAccount(w http.ResponseWriter, r *http.Request) error {
	return a.toggleAccount(w, r, a.svc.Admin.Enable)
}

func (a *API) toggleAccount(w http.ResponseWriter, r *http.Request, act func(ctx context.Context, actor, id uuid.UUID) (*user.User, error)) error {
	id, err := pathID(r, "userId", "Account")
	if err != nil {
		return err
	}
	u, err := act(r.Context(), me(r), id)
	if err != nil {
		return err
	}
	return details(w, http.StatusOK, toAccount(u))
}
