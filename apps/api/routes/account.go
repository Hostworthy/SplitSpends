package routes

import (
	"net/http"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/user"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/Hostworthy/SplitSpends/packages/middleware"
	"github.com/Hostworthy/SplitSpends/packages/platform"
)

func (a *API) requestCode(w http.ResponseWriter, r *http.Request) error {
	in, err := decode[platform.OTPRequest](r)
	if err != nil {
		return err
	}
	if err := a.svc.Auth.RequestCode(r.Context(), in.Email, in.InviteToken, middleware.ByAddress(a.cfg.ProxyHops)(r)); err != nil {
		return err
	}
	return details(w, http.StatusAccepted, platform.OTPSent{Sent: true, ExpiresInSeconds: int(a.svc.Auth.CodeTTL().Seconds())})
}

func (a *API) verifyCode(w http.ResponseWriter, r *http.Request) error {
	in, err := decode[platform.OTPVerify](r)
	if err != nil {
		return err
	}
	login, err := a.svc.Auth.Verify(r.Context(), in.Email, in.Code, in.Name, r.UserAgent())
	if err != nil {
		return err
	}
	return details(w, http.StatusOK, platform.LoginResult{Token: login.Token, ExpiresAt: stamp(*login.Session.ExpiresAt),
		User: toMe(login.User), IsNew: login.IsNew})
}

func (a *API) logout(w http.ResponseWriter, r *http.Request) error {
	if err := a.svc.Auth.Logout(r.Context(), middleware.Principal(r.Context())); err != nil {
		return err
	}
	return noContent(w)
}

func (a *API) getMe(w http.ResponseWriter, r *http.Request) error {
	u, err := a.svc.Users.Get(r.Context(), me(r))
	if err != nil {
		return err
	}
	return details(w, http.StatusOK, toMe(u))
}

func (a *API) patchMe(w http.ResponseWriter, r *http.Request) error {
	in, err := decode[platform.MePatch](r)
	if err != nil {
		return err
	}
	if in.Gravatar != nil && *in.Gravatar && !a.cfg.Gravatar {
		return errs.Validation("gravatar", "Gravatar pictures are turned off on this server.")
	}
	p := user.Patch{Name: in.Name, DefaultCurrency: in.DefaultCurrency, Timezone: in.Timezone, AvatarColor: in.AvatarColor, Gravatar: in.Gravatar}
	if in.Notifications != nil {
		n := user.NotificationsPatch{SwitchesPatch: user.SwitchesPatch(in.Notifications.NotificationSwitchesPatch)}
		if in.Notifications.Push != nil {
			push := user.SwitchesPatch(*in.Notifications.Push)
			n.Push = &push
		}
		p.Notifications = &n
	}
	u, err := a.svc.Users.Update(r.Context(), me(r), p)
	if err != nil {
		return err
	}
	return details(w, http.StatusOK, toMe(u))
}

func (a *API) myBalances(w http.ResponseWriter, r *http.Request) error {
	sheet, err := a.svc.Balances.For(r.Context(), me(r))
	if err != nil {
		return err
	}
	net, owed, owing := sheet.Totals()
	return details(w, http.StatusOK, platform.MyBalances{Balances: toBalances(net), Owed: toBalances(owed), Owing: toBalances(owing)})
}

func (a *API) listTokens(w http.ResponseWriter, r *http.Request) error {
	tokens, err := a.svc.Auth.Tokens(r.Context(), me(r))
	if err != nil {
		return err
	}
	out := make([]platform.Token, 0, len(tokens))
	for _, t := range tokens {
		out = append(out, toToken(t, ""))
	}
	return list(w, out, len(out))
}

func (a *API) createToken(w http.ResponseWriter, r *http.Request) error {
	in, err := decode[platform.TokenCreate](r)
	if err != nil {
		return err
	}
	t, err := a.svc.Auth.CreateToken(r.Context(), middleware.Principal(r.Context()), in.Name)
	if err != nil {
		return err
	}
	return details(w, http.StatusCreated, toToken(t.Session, t.Token))
}

func (a *API) deleteToken(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "tokenId", "Token")
	if err != nil {
		return err
	}
	if err := a.svc.Auth.RevokeToken(r.Context(), middleware.Principal(r.Context()), id); err != nil {
		return err
	}
	return noContent(w)
}
