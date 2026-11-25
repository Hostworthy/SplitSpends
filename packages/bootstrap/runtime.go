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
	// quotas and clock let the jobs remove budgets whose window has ended.
	quotas *repositories.Quotas
	clock  clock.Clock
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
	if cfg.baseURLDefault {
		log.Warn("bootstrap.config.base-url.default", "baseUrl", cfg.BaseURL)
	}
	pool, err := database.Connect(ctx, cfg.DatabaseURL, cfg.DatabaseConns)
	if err != nil {
		return nil, err
	}
	if err := database.Migrate(ctx, pool, log); err != nil {
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
		var s3 *storage.S3
		s3, err = storage.NewS3(storage.S3Config{Bucket: cfg.S3Bucket, Region: cfg.S3Region, Endpoint: cfg.S3Endpoint,
			AccessKey: cfg.S3AccessKey, SecretKey: cfg.S3SecretKey, Prefix: cfg.S3Prefix})
		if err == nil {
			blobs = s3
			if !s3.Secure() {
				log.Warn("bootstrap.storage.insecure", "endpoint", cfg.S3Endpoint)
			}
		}
	case "postgres":
		blobs = repositories.NewBlobs(db)
	default:
		blobs, err = storage.NewLocal(cfg.DataDir)
	}
	if err != nil {
		notifier.Close()
		pool.Close()
		return nil, err
	}
	log.Info("bootstrap.storage.ready", "store", cfg.BlobStore)

	var reader ports.Assistant
	switch cfg.AIProvider {
	case "anthropic":
		reader = ai.NewAnthropic(cfg.AIAPIKey, cfg.AIBaseURL, cfg.AIModel, cfg.AIEffort)
	case "openai":
		reader = ai.NewOpenAICompatible(cfg.AIBaseURL, cfg.AIAPIKey, cfg.AIModel)
	}
	if reader != nil {
		log.Info("bootstrap.ai.enabled", "provider", cfg.AIProvider)
	}

	services := Wire(cfg, Adapters{DB: db, Email: notifier, Blobs: blobs, Reader: reader,
		Push: pushSender(ctx, db, cfg, log), Clock: clock.System{}, Log: log})
	if open, err := services.Auth.Unclaimed(ctx); err == nil && open {
		log.Warn("auth.bootstrap.open", "reason", "no account yet and ADMIN_EMAIL is empty: whoever signs in first becomes the administrator, whatever SIGNUP_MODE says")
	}
	return &Runtime{Config: cfg, Log: log, db: db, notifier: notifier, Services: services}, nil
}

// Adapters are the infrastructure services run on. Tests fill the ones they
// need with fakes.
type Adapters struct {
	DB *database.DB
	// Email sends login codes, invites and notifications.
	Email  ports.Notifier
	Blobs  ports.BlobStore
	Reader ports.Assistant
	// Push delivers Web Push and knows its VAPID key; nil disables push.
	Push push.Sender
	// Subscriptions stores push devices; nil keeps them in DB.
	Subscriptions push.Repository
	Clock         clock.Clock
	// Log is where services log; nil discards.
	Log *slog.Logger
}

// Wire builds services on adapters. Notifications go out by email and, when
// a.Push is set, by Web Push.
func Wire(cfg *Config, a Adapters) *Services {
	db, c, log := a.DB, a.Clock, a.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	subscriptions := a.Subscriptions
	if subscriptions == nil {
		subscriptions = repositories.NewPushSubscriptions(db)
	}
	pushes := push.NewService(subscriptions, a.Push, c,
		push.Settings{AppName: cfg.AppName, Link: cfg.BaseURL, AllowPrivate: cfg.PushAllowPrivate})
	var p pusher
	if pushes.Enabled() {
		p = pushes
	}
	notifier := newFanout(a.Email, p, log, cfg.Serverless)
	blobs, reader := a.Blobs, a.Reader
	tx := database.NewTx(db)
	// The repositories more than one service reads; the others are made
	// where they are passed.
	store := struct {
		users    *repositories.Users
		groups   *repositories.Groups
		friends  *repositories.Friends
		expenses *repositories.Expenses
		receipts *repositories.Receipts
	}{repositories.NewUsers(db), repositories.NewGroups(db), repositories.NewFriends(db), repositories.NewExpenses(db), repositories.NewReceipts(db)}

	policy := auth.DefaultPolicy()
	policy.Secret, policy.Mode, policy.AllowedDomains, policy.AdminEmail = cfg.Secret, cfg.SignupMode, cfg.AllowedDomains, cfg.AdminEmail
	if cfg.SessionTTL > 0 {
		policy.SessionTTL = cfg.SessionTTL
	}

	quotas := repositories.NewQuotas(db)
	users := user.NewService(store.users, notifier, quotas, tx, c, log, user.Settings{Gravatar: cfg.Gravatar})
	act := activity.NewService(repositories.NewActivities(db), c)
	balances := balance.NewService(store.expenses)
	groups := group.NewService(group.Deps{Repo: store.groups, Users: users, Balances: balances, Activity: act, Notifier: notifier,
		Quota: quotas, Tx: tx, Clock: c, BaseURL: cfg.BaseURL})
	friends := friend.NewService(store.friends, users, balances, act, tx, c)
	receipts := receipt.NewService(store.receipts, blobs, reader, tx, c)
	categories := category.NewService(repositories.NewCategories(db), c)
	expenses := expense.NewService(expense.Deps{Repo: store.expenses, Comments: repositories.NewComments(db), Groups: groups,
		Connections: friends, Receipts: store.receipts, Catalog: categories, Users: users, Activity: act, Notifier: notifier, Tx: tx, Clock: c, Log: log, BaseURL: cfg.BaseURL})
	// Rates only convert AI drafts read in another currency, so nothing is
	// fetched unless AI is on.
	var source rates.Source
	if cfg.RatesURL != "" && reader != nil {
		source = exchange.New(cfg.RatesURL)
	}
	exchangeRates := rates.NewService(repositories.NewRates(db), source, c, log)
	invites := invite.NewService(repositories.NewInvites(db), groups, store.groups, users, tx, c, cfg.BaseURL)
	reminders := reminder.NewService(reminder.Deps{Repo: repositories.NewReminders(db), Groups: groups, Connections: friends,
		Users: users, Balances: balances, Activity: act, Notifier: notifier, Quota: quotas, Tx: tx, Clock: c, BaseURL: cfg.BaseURL})
	administration := admin.NewService(admin.Deps{Settings: repositories.NewSignupSettings(db), Requests: repositories.NewSignupRequests(db),
		Users: store.users, Accounts: store.users, Credentials: repositories.NewCredentials(db), Notifier: notifier, Tx: tx, Clock: c, Log: log,
		Defaults: policy.Signup(), BaseURL: cfg.BaseURL, AppName: cfg.AppName})
	authentication := auth.NewService(policy, repositories.NewChallenges(db), repositories.NewSessions(db), store.users, invites, notifier, tx, c, log)
	authentication.Use(administration)
	assistant := assist.NewService(reader, users, groups, store.friends, receipts, exchangeRates, c)
	statistics := stats.NewService(repositories.NewStats(db), groups, users, categories, c)
	transfers := transfer.NewService(users, store.friends, groups, expenses, categories, tx, tx)
	return &Services{
		Auth:       authentication,
		Users:      users,
		Friends:    friends,
		Groups:     groups,
		Expenses:   expenses,
		Balances:   balances,
		Activity:   act,
		Receipts:   receipts,
		Assist:     assistant,
		Stats:      statistics,
		Transfer:   transfers,
		Reminders:  reminders,
		Invites:    invites,
		Rates:      exchangeRates,
		Push:       pushes,
		Categories: categories,
		Admin:      administration,
		notifier:   notifier,
		quotas:     quotas,
		clock:      c,
	}
}

// RunJobsOnce creates due recurring expenses and removes expired
// credentials, receipts no expense uses and budgets whose window has ended. Serverless hosts call it from a
// scheduler via /internal/jobs. Every job runs even when another fails; the
// error joins their failures, which are also logged.
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
	removed, receipts := rt.Services.Receipts.Cleanup(ctx)
	if removed > 0 {
		rt.Log.Info("jobs.receipts.cleanup.completed", "removed", removed)
	}
	if receipts != nil {
		rt.Log.Error("jobs.receipts.cleanup.failed", "error", receipts)
	}
	pruned, quotas := rt.Services.quotas.Prune(ctx, rt.Services.clock.Now())
	if pruned > 0 {
		rt.Log.Info("jobs.quotas.cleanup.completed", "removed", pruned)
	}
	if quotas != nil {
		rt.Log.Error("jobs.quotas.cleanup.failed", "error", quotas)
	}
	return errors.Join(recurring, sessions, invites, requests, receipts, quotas)
}

// passes bounds the recurring passes of one run; each takes on up to
// expense.Batch series.
const passes = 50

// recurring runs passes until one books or ends no series, logging each
// series that failed. A pass that only ended series goes on, since more
// may be due behind them.
func (rt *Runtime) recurring(ctx context.Context) error {
	var failed []error
	for pass := 1; ; pass++ {
		done, err := rt.Services.Expenses.RunRecurring(ctx)
		if err != nil {
			failed = append(failed, err)
			rt.failed(err)
		}
		if done.Copies > 0 {
			rt.Log.Info("jobs.recurring.created", "count", done.Copies)
		}
		if done.Series == 0 {
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
