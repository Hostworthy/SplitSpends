package bootstrap

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/ai"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/activity"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/admin"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/assist"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/auth"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/balance"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/category"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/expense"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/friend"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/group"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/invite"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/push"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/rates"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/receipt"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/reminder"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/stats"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/transfer"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/user"
	"github.com/Hostworthy/SplitSpends/packages/core/ports"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/clock"
	"github.com/Hostworthy/SplitSpends/packages/database"
	"github.com/Hostworthy/SplitSpends/packages/database/repositories"
	"github.com/Hostworthy/SplitSpends/packages/exchange"
	"github.com/Hostworthy/SplitSpends/packages/notifications"
	"github.com/Hostworthy/SplitSpends/packages/storage"
)

// Services are the core use cases the transports call.
type Services struct {
	Auth      *auth.Service
	Users     *user.Service
	Friends   *friend.Service
	Groups    *group.Service
	Expenses  *expense.Service
	Balances  *balance.Service
	Activity  *activity.Service
	Receipts  *receipt.Service
	Assist    *assist.Service
	Stats     *stats.Service
	Transfer  *transfer.Service
	Reminders *reminder.Service
	Invites   *invite.Service
	Rates     *rates.Service
	Push      *push.Service
	// Categories are the built-in plus user-defined expense categories.
	Categories *category.Service
	// Admin runs sign-up settings, approvals, roles and disabled accounts.
	Admin *admin.Service

	notifier *fanout
}

// Runtime is a fully wired application.
type Runtime struct {
	Config   *Config
	Log      *slog.Logger
	Services *Services
	db       *database.DB
	notifier *notifications.Notifier
}

// Load connects, migrates and wires every service.
func Load(ctx context.Context, cfg *Config, log *slog.Logger) (*Runtime, error) {
	pool, err := database.Connect(ctx, cfg.DatabaseURL, cfg.DatabaseConns)
	if err != nil {
		return nil, err
	}
	if err := database.Migrate(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	log.Info("bootstrap.database.migrated")
	db := &database.DB{Pool: pool}

	notifier, err := notifications.New(notifications.Config{
		Host: cfg.SMTPHost, Port: cfg.SMTPPort, Username: cfg.SMTPUsername, Password: cfg.SMTPPassword,
		From: cfg.SMTPFrom, Security: cfg.SMTPSecurity, AppName: cfg.AppName, BaseURL: cfg.BaseURL, Sync: cfg.Serverless,
	}, log)
	if err != nil {
		pool.Close()
		return nil, err
	}
	var blobs ports.BlobStore
	switch cfg.BlobStore {
	case "s3":
		blobs, err = storage.NewS3(storage.S3Config{Bucket: cfg.S3Bucket, Region: cfg.S3Region, Endpoint: cfg.S3Endpoint,
			AccessKey: cfg.S3AccessKey, SecretKey: cfg.S3SecretKey, Prefix: cfg.S3Prefix})
	case "postgres":
		blobs = repositories.NewBlobs(db)
	default:
		blobs, err = storage.NewLocal(cfg.DataDir)
	}
	if err != nil {
		pool.Close()
		return nil, err
	}
	log.Info("bootstrap.storage.ready", "store", cfg.BlobStore)

	var reader ports.ReceiptReader
	switch cfg.AIProvider {
	case "anthropic":
		reader = ai.NewAnthropic(cfg.AIAPIKey, cfg.AIBaseURL, cfg.AIModel, cfg.AIEffort)
	case "openai":
		reader = ai.NewOpenAICompatible(cfg.AIBaseURL, cfg.AIAPIKey, cfg.AIModel)
	}
	if reader != nil {
		log.Info("bootstrap.ai.enabled", "provider", cfg.AIProvider)
	}

	sender := pushSender(ctx, db, cfg, log)
	services := WireWithPush(db, notifier, blobs, reader, sender, clock.System{}, cfg, log)
	if open, err := services.Auth.Unclaimed(ctx); err == nil && open {
		log.Warn("auth.bootstrap.open", "reason", "no account yet and ADMIN_EMAIL is empty: whoever signs in first becomes the administrator, whatever SIGNUP_MODE says")
	}
	return &Runtime{Config: cfg, Log: log, db: db, notifier: notifier, Services: services}, nil
}

// Wire builds services from adapters, without push. Tests call it with
// fakes.
func Wire(db *database.DB, notifier ports.Notifier, blobs ports.BlobStore, reader ports.ReceiptReader, c clock.Clock, cfg *Config) *Services {
	return WireWithPush(db, notifier, blobs, reader, nil, c, cfg, slog.New(slog.DiscardHandler))
}

// WireWithPush builds services and sends notifications by email (notifier)
// and Web Push (sender, nil to disable push).
func WireWithPush(db *database.DB, email ports.Notifier, blobs ports.BlobStore, reader ports.ReceiptReader, sender push.Sender,
	c clock.Clock, cfg *Config, log *slog.Logger) *Services {
	pushes := push.NewService(repositories.NewPushSubscriptions(db), sender, c,
		push.Settings{PublicKey: cfg.VAPIDPublicKey, AppName: cfg.AppName, Link: cfg.BaseURL, AllowPrivate: cfg.PushAllowPrivate})
	var p pusher
	if pushes.Enabled() {
		p = pushes
	}
	notifier := newFanout(email, p, log, cfg.Serverless)
	tx := database.NewTx(db)
	userRepo := repositories.NewUsers(db)
	groupRepo := repositories.NewGroups(db)
	friendRepo := repositories.NewFriends(db)
	expenseRepo := repositories.NewExpenses(db)
	receiptRepo := repositories.NewReceipts(db)

	policy := auth.DefaultPolicy()
	policy.Secret, policy.Mode, policy.AllowedDomains, policy.AdminEmail = cfg.Secret, cfg.SignupMode, cfg.AllowedDomains, cfg.AdminEmail
	if cfg.SessionTTL > 0 {
		policy.SessionTTL = cfg.SessionTTL
	}

	quotas := repositories.NewQuotas(db)
	users := user.NewService(userRepo, notifier, quotas, c)
	act := activity.NewService(repositories.NewActivities(db), c)
	balances := balance.NewService(expenseRepo)
	groups := group.NewService(groupRepo, users, balances, act, notifier, quotas, tx, c, cfg.BaseURL)
	friends := friend.NewService(friendRepo, users, balances, act, tx, c)
	receipts := receipt.NewService(receiptRepo, blobs, reader, c)
	categories := category.NewService(repositories.NewCategories(db), c)
	expenses := expense.NewService(expense.Deps{Repo: expenseRepo, Comments: repositories.NewComments(db), Groups: groupRepo,
		Connections: friends, Receipts: receiptRepo, Catalog: categories, Users: users, Activity: act, Notifier: notifier, Tx: tx, Clock: c, BaseURL: cfg.BaseURL})
	// Rates only convert AI drafts read in another currency, so nothing is
	// fetched unless AI is on.
	var source rates.Source
	if cfg.RatesURL != "" && reader != nil {
		source = exchange.New(cfg.RatesURL)
	}
	fx := rates.NewService(repositories.NewRates(db), source, c)
	invites := invite.NewService(repositories.NewInvites(db), groups, groupRepo, users, tx, c, cfg.BaseURL)
	reminders := reminder.NewService(reminder.Deps{Repo: repositories.NewReminders(db), Groups: groups, Connections: friends,
		Users: users, Balances: balances, Activity: act, Notifier: notifier, Quota: quotas, Tx: tx, Clock: c, BaseURL: cfg.BaseURL})
	administration := admin.NewService(admin.Deps{Settings: repositories.NewSignupSettings(db), Requests: repositories.NewSignupRequests(db),
		Users: userRepo, Accounts: userRepo, Credentials: repositories.NewCredentials(db), Notifier: notifier, Tx: tx, Clock: c,
		Defaults: policy.Signup(), BaseURL: cfg.BaseURL, AppName: cfg.AppName})
	authn := auth.NewService(policy, repositories.NewChallenges(db), repositories.NewSessions(db), userRepo, invites, notifier, tx, c)
	authn.Use(administration)
	return &Services{
		Admin:    administration,
		Rates:    fx,
		Auth:     authn,
		Users:    users,
		Friends:  friends,
		Groups:   groups,
		Balances: balances,
		Activity: act,
		Expenses: expenses,
		Transfer: transfer.NewService(users, friendRepo, groups, expenses, categories, tx, tx),
		Receipts: receipts,
		Assist:   assist.NewService(reader, users, groups, friendRepo, receipts, fx, c),
		Stats:    stats.NewService(repositories.NewStats(db), groupRepo, users, categories),

		Reminders:  reminders,
		Invites:    invites,
		Push:       pushes,
		notifier:   notifier,
		Categories: categories,
	}
}

// RunJobsOnce creates due recurring expenses and removes expired
// credentials. Serverless hosts call it from a scheduler via /internal/jobs.
// Every job runs even when another fails; the error joins their failures,
// which are also logged.
func (rt *Runtime) RunJobsOnce(ctx context.Context) error {
	recurring := rt.recurring(ctx)
	// Rates are a convenience for AI drafts; an outage never fails the jobs.
	if err := rt.Services.Rates.Refresh(ctx); err != nil {
		rt.Log.Warn("jobs.rates.failed", "error", err)
	}
	sessions := rt.Services.Auth.Cleanup(ctx)
	if sessions != nil {
		rt.Log.Error("jobs.sessions.cleanup.failed", "error", sessions)
	}
	invites := rt.Services.Invites.Cleanup(ctx)
	if invites != nil {
		rt.Log.Error("jobs.invites.cleanup.failed", "error", invites)
	}
	requests := rt.Services.Admin.Cleanup(ctx)
	if requests != nil {
		rt.Log.Error("jobs.requests.cleanup.failed", "error", requests)
	}
	return errors.Join(recurring, sessions, invites, requests)
}

// passes bounds the recurring passes of one run; each books up to 100 series.
const passes = 50

// recurring runs passes until nothing is due, logging each series that
// failed.
func (rt *Runtime) recurring(ctx context.Context) error {
	var failed []error
	for pass := 1; ; pass++ {
		n, err := rt.Services.Expenses.RunRecurring(ctx)
		if err != nil {
			failed = append(failed, err)
			rt.failed(err)
		}
		if n > 0 {
			rt.Log.Info("jobs.recurring.created", "count", n)
		}
		if n == 0 {
			break
		}
		if pass == passes {
			rt.Log.Warn("jobs.recurring.capped", "passes", passes)
			break
		}
	}
	return errors.Join(failed...)
}

// failed logs an error from RunRecurring: one line per skipped series.
func (rt *Runtime) failed(err error) {
	list := []error{err}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		list = joined.Unwrap()
	}
	for _, e := range list {
		var s *expense.Skipped
		if errors.As(e, &s) {
			rt.Log.Error("jobs.recurring.skipped", "expense", s.ID, "error", s.Err)
			continue
		}
		rt.Log.Error("jobs.recurring.failed", "error", e)
	}
}

// RunJobs runs RunJobsOnce every 15 minutes until ctx ends.
func (rt *Runtime) RunJobs(ctx context.Context) {
	tick := func() { _ = rt.RunJobsOnce(ctx) }
	tick()
	t := time.NewTicker(15 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			tick()
		}
	}
}

// Ping checks database connectivity.
func (rt *Runtime) Ping(ctx context.Context) error { return rt.db.Pool.Ping(ctx) }

// Close releases resources.
func (rt *Runtime) Close() {
	rt.Services.notifier.Close()
	rt.notifier.Close()
	rt.db.Pool.Close()
}
