package bootstrap

import (
	"context"
	"log/slog"
	"time"

	"github.com/atb00ker/splitspends/backend/internal/ai"
	"github.com/atb00ker/splitspends/backend/internal/core/domains/activity"
	"github.com/atb00ker/splitspends/backend/internal/core/domains/assist"
	"github.com/atb00ker/splitspends/backend/internal/core/domains/auth"
	"github.com/atb00ker/splitspends/backend/internal/core/domains/balance"
	"github.com/atb00ker/splitspends/backend/internal/core/domains/expense"
	"github.com/atb00ker/splitspends/backend/internal/core/domains/friend"
	"github.com/atb00ker/splitspends/backend/internal/core/domains/group"
	"github.com/atb00ker/splitspends/backend/internal/core/domains/receipt"
	"github.com/atb00ker/splitspends/backend/internal/core/domains/stats"
	"github.com/atb00ker/splitspends/backend/internal/core/domains/user"
	"github.com/atb00ker/splitspends/backend/internal/core/ports"
	"github.com/atb00ker/splitspends/backend/internal/core/shared/clock"
	"github.com/atb00ker/splitspends/backend/internal/database"
	"github.com/atb00ker/splitspends/backend/internal/database/repositories"
	"github.com/atb00ker/splitspends/backend/internal/notifications"
	"github.com/atb00ker/splitspends/backend/internal/storage"
)

// Services are the core use cases the transports call.
type Services struct {
	Auth     *auth.Service
	Users    *user.Service
	Friends  *friend.Service
	Groups   *group.Service
	Expenses *expense.Service
	Balances *balance.Service
	Activity *activity.Service
	Receipts *receipt.Service
	Assist   *assist.Service
	Stats    *stats.Service
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
		From: cfg.SMTPFrom, Security: cfg.SMTPSecurity, AppName: cfg.AppName, BaseURL: cfg.BaseURL,
	}, log)
	if err != nil {
		pool.Close()
		return nil, err
	}
	blobs, err := storage.NewLocal(cfg.DataDir)
	if err != nil {
		pool.Close()
		return nil, err
	}

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

	return &Runtime{Config: cfg, Log: log, db: db, notifier: notifier,
		Services: Wire(db, notifier, blobs, reader, clock.System{}, cfg)}, nil
}

// Wire builds services from adapters. Tests call it with fakes.
func Wire(db *database.DB, notifier ports.Notifier, blobs ports.BlobStore, reader ports.ReceiptReader, c clock.Clock, cfg *Config) *Services {
	tx := database.NewTx(db)
	userRepo := repositories.NewUsers(db)
	groupRepo := repositories.NewGroups(db)
	friendRepo := repositories.NewFriends(db)
	expenseRepo := repositories.NewExpenses(db)
	receiptRepo := repositories.NewReceipts(db)

	policy := auth.DefaultPolicy()
	policy.Secret, policy.Mode, policy.AllowedDomains = cfg.Secret, cfg.SignupMode, cfg.AllowedDomains
	if cfg.SessionTTL > 0 {
		policy.SessionTTL = cfg.SessionTTL
	}

	users := user.NewService(userRepo, notifier, c)
	act := activity.NewService(repositories.NewActivities(db), c)
	balances := balance.NewService(expenseRepo)
	groups := group.NewService(groupRepo, users, balances, act, notifier, tx, c, cfg.BaseURL)
	friends := friend.NewService(friendRepo, users, balances, act, tx, c)
	return &Services{
		Auth:     auth.NewService(policy, repositories.NewChallenges(db), repositories.NewSessions(db), userRepo, notifier, tx, c),
		Users:    users,
		Friends:  friends,
		Groups:   groups,
		Balances: balances,
		Activity: act,
		Expenses: expense.NewService(expense.Deps{Repo: expenseRepo, Comments: repositories.NewComments(db), Groups: groupRepo,
			Connections: friends, Receipts: receiptRepo, Users: users, Activity: act, Notifier: notifier, Tx: tx, Clock: c, BaseURL: cfg.BaseURL}),
		Receipts: receipt.NewService(receiptRepo, blobs, reader, c),
		Assist:   assist.NewService(reader, users, groups, friendRepo, c),
		Stats:    stats.NewService(repositories.NewStats(db), groupRepo, users),
	}
}

// RunJobs runs periodic maintenance until ctx ends: recurring expenses and
// credential cleanup.
func (rt *Runtime) RunJobs(ctx context.Context) {
	tick := func() {
		for pass := 0; pass < 50; pass++ {
			n, err := rt.Services.Expenses.RunRecurring(ctx)
			if err != nil {
				rt.Log.Error("jobs.recurring.failed", "error", err)
				break
			}
			if n > 0 {
				rt.Log.Info("jobs.recurring.created", "count", n)
			}
			if n == 0 {
				break
			}
		}
		if err := rt.Services.Auth.Cleanup(ctx); err != nil {
			rt.Log.Error("jobs.cleanup.failed", "error", err)
		}
	}
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
	rt.notifier.Close()
	rt.db.Pool.Close()
}
