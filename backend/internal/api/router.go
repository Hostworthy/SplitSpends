package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/Hostworthy/SplitSpends/backend/internal/bootstrap"
	"github.com/Hostworthy/SplitSpends/backend/internal/middleware"
	"github.com/Hostworthy/SplitSpends/backend/internal/platform"
)

// Deps are what the API needs from the runtime.
type Deps struct {
	Services *bootstrap.Services
	Config   *bootstrap.Config
	Log      *slog.Logger
	// Ping checks dependencies for /health.
	Ping func(ctx context.Context) error
	// Jobs runs background maintenance once, for /internal/jobs.
	Jobs func(ctx context.Context) error
}

// API holds handler dependencies.
type API struct {
	svc  *bootstrap.Services
	cfg  *bootstrap.Config
	log  *slog.Logger
	ping func(ctx context.Context) error
	jobs func(ctx context.Context) error
}

// New builds the API.
func New(d Deps) *API {
	return &API{svc: d.Services, cfg: d.Config, log: d.Log, ping: d.Ping, jobs: d.Jobs}
}

// Routes returns the API handler mounted at platform.Prefix plus /health.
func (a *API) Routes() *http.ServeMux {
	mux := http.NewServeMux()
	authed := func(h handler) http.Handler { return middleware.RequireAuth(a.svc.Auth, a.log, a.wrap(h)) }
	public := func(h handler) http.Handler { return a.wrap(h) }
	otpLimit := middleware.NewLimiter(20, time.Minute)
	limited := func(h handler) http.Handler { return middleware.Limit(otpLimit, a.log, a.cfg.TrustProxy, a.wrap(h)) }
	p := platform.Prefix

	mux.Handle("GET /health", public(a.health))
	mux.Handle("GET "+p+"/config", public(a.config))
	if a.cfg.JobsSecret != "" && a.jobs != nil {
		mux.Handle("POST /internal/jobs", public(a.runJobs))
	}

	mux.Handle("POST "+p+"/auth/otp/request", limited(a.requestCode))
	mux.Handle("POST "+p+"/auth/otp/verify", limited(a.verifyCode))
	mux.Handle("POST "+p+"/auth/logout", authed(a.logout))

	mux.Handle("GET "+p+"/me", authed(a.getMe))
	mux.Handle("PATCH "+p+"/me", authed(a.patchMe))
	mux.Handle("GET "+p+"/me/balances", authed(a.myBalances))
	mux.Handle("GET "+p+"/me/tokens", authed(a.listTokens))
	mux.Handle("POST "+p+"/me/tokens", authed(a.createToken))
	mux.Handle("DELETE "+p+"/me/tokens/{tokenId}", authed(a.deleteToken))

	mux.Handle("GET "+p+"/friends", authed(a.listFriends))
	mux.Handle("POST "+p+"/friends", authed(a.addFriend))
	mux.Handle("GET "+p+"/friends/{userId}", authed(a.getFriend))
	mux.Handle("PATCH "+p+"/friends/{userId}", authed(a.inviteFriend))
	mux.Handle("DELETE "+p+"/friends/{userId}", authed(a.removeFriend))
	mux.Handle("POST "+p+"/import", authed(a.importBundle))

	mux.Handle("GET "+p+"/groups", authed(a.listGroups))
	mux.Handle("POST "+p+"/groups", authed(a.createGroup))
	mux.Handle("GET "+p+"/groups/{groupId}", authed(a.getGroup))
	mux.Handle("PATCH "+p+"/groups/{groupId}", authed(a.patchGroup))
	mux.Handle("DELETE "+p+"/groups/{groupId}", authed(a.deleteGroup))
	mux.Handle("POST "+p+"/groups/{groupId}/members", authed(a.addMember))
	mux.Handle("DELETE "+p+"/groups/{groupId}/members/{userId}", authed(a.removeMember))
	mux.Handle("GET "+p+"/groups/{groupId}/balances", authed(a.groupBalances))
	mux.Handle("GET "+p+"/groups/{groupId}/export", authed(a.exportGroup))

	mux.Handle("GET "+p+"/expenses", authed(a.listExpenses))
	mux.Handle("POST "+p+"/expenses", authed(a.createExpense))
	mux.Handle("GET "+p+"/expenses/{expenseId}", authed(a.getExpense))
	mux.Handle("PUT "+p+"/expenses/{expenseId}", authed(a.updateExpense))
	mux.Handle("DELETE "+p+"/expenses/{expenseId}", authed(a.deleteExpense))
	mux.Handle("POST "+p+"/expenses/{expenseId}/restore", authed(a.restoreExpense))
	mux.Handle("GET "+p+"/expenses/{expenseId}/comments", authed(a.listComments))
	mux.Handle("POST "+p+"/expenses/{expenseId}/comments", authed(a.addComment))
	mux.Handle("DELETE "+p+"/comments/{commentId}", authed(a.deleteComment))
	mux.Handle("POST "+p+"/payments", authed(a.createPayment))

	mux.Handle("GET "+p+"/activity", authed(a.listActivity))
	mux.Handle("GET "+p+"/stats", authed(a.stats))

	mux.Handle("POST "+p+"/receipts", authed(a.uploadReceipt))
	mux.Handle("GET "+p+"/receipts/{receiptId}", authed(a.getReceipt))
	mux.Handle("POST "+p+"/receipts/{receiptId}/scan", authed(a.scanReceipt))
	mux.Handle("POST "+p+"/ai/expense-draft", authed(a.draftExpense))

	mux.Handle(p+"/", public(a.notFound))
	return mux
}
