package api

import (
	"context"
	"crypto/subtle"
	"net/http"
	"time"

	"github.com/atb00ker/splitspends/backend/internal/core/domains/expense"
	"github.com/atb00ker/splitspends/backend/internal/core/shared/errs"
	"github.com/atb00ker/splitspends/backend/internal/core/shared/money"
	"github.com/atb00ker/splitspends/backend/internal/middleware"
	"github.com/atb00ker/splitspends/backend/internal/platform"
)

func (a *API) health(w http.ResponseWriter, r *http.Request) error {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if a.ping != nil {
		if err := a.ping(ctx); err != nil {
			middleware.WriteJSON(w, http.StatusServiceUnavailable, platform.Health{OK: false})
			return nil
		}
	}
	middleware.WriteJSON(w, http.StatusOK, platform.Health{OK: true})
	return nil
}

func (a *API) config(w http.ResponseWriter, _ *http.Request) error {
	cfg := platform.Config{AppName: a.cfg.AppName, AIEnabled: a.svc.Receipts.AIEnabled(), SignupMode: string(a.cfg.SignupMode)}
	for _, c := range money.Currencies() {
		cfg.Currencies = append(cfg.Currencies, platform.Currency{Code: c.Code, Name: c.Name, Symbol: c.Symbol, Exponent: c.Exponent})
	}
	for _, c := range expense.Categories() {
		cfg.Categories = append(cfg.Categories, platform.Category{ID: c.ID, Name: c.Name, Group: c.Group, Icon: c.Icon})
	}
	w.Header().Set("Cache-Control", "public, max-age=300")
	return details(w, http.StatusOK, cfg)
}

// runJobs lets a scheduler (EventBridge, Cloud Scheduler, cron) trigger
// maintenance on serverless hosts. It requires the JOBS_SECRET bearer token.
func (a *API) runJobs(w http.ResponseWriter, r *http.Request) error {
	token := middleware.BearerToken(r)
	if subtle.ConstantTimeCompare([]byte(token), []byte(a.cfg.JobsSecret)) != 1 {
		return errs.Unauthorized("Invalid jobs token.")
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	if err := a.jobs(ctx); err != nil {
		return err
	}
	return noContent(w)
}

func (a *API) notFound(http.ResponseWriter, *http.Request) error {
	return errs.NotFound("Endpoint")
}
