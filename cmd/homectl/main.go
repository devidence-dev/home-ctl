// Package main is the entry point for the homectl service.
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/px1/home-ctl/internal/chores"
	"github.com/px1/home-ctl/internal/clock"
	"github.com/px1/home-ctl/internal/config"
	"github.com/px1/home-ctl/internal/flow"
	"github.com/px1/home-ctl/internal/household"
	"github.com/px1/home-ctl/internal/i18n"
	"github.com/px1/home-ctl/internal/reminders"
	"github.com/px1/home-ctl/internal/scheduler"
	"github.com/px1/home-ctl/internal/shopping"
	"github.com/px1/home-ctl/internal/storage/sqlite"
	"github.com/px1/home-ctl/internal/telegram"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	logger.Info("starting homectl service")

	configDir := os.Getenv("CONFIG_DIR")
	if configDir == "" {
		configDir = "config"
	}
	localesDir := os.Getenv("LOCALES_DIR")
	if localesDir == "" {
		localesDir = "locales"
	}

	cfg, err := config.Load(config.Options{
		ConfigDir:  configDir,
		LocalesDir: localesDir,
		Getenv:     os.Getenv,
	})
	if err != nil {
		logger.Error("failed to load configuration", "err", err)
		os.Exit(1)
	}

	clk := clock.NewWithLocation(cfg.Location)

	dbDir := filepath.Dir(cfg.App.DatabasePath)
	if err := os.MkdirAll(dbDir, 0o750); err != nil {
		logger.Error("failed to create database directory", "dir", dbDir, "err", err)
		os.Exit(1)
	}

	db, err := sqlite.Open(cfg.App.DatabasePath)
	if err != nil {
		logger.Error("failed to open database", "path", cfg.App.DatabasePath, "err", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()

	if err := sqlite.RunMigrations(db); err != nil {
		logger.Error("failed to run database migrations", "err", err)
		os.Exit(1)
	}

	bundle, err := i18n.NewBundle(localesDir, cfg.App.DefaultLocale, i18n.DefaultRequiredKeys, logger)
	if err != nil {
		logger.Error("failed to initialize i18n bundle", "err", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	hhStore := sqlite.NewHouseholdStore(db)
	shStore := sqlite.NewShoppingStore(db)
	chStore := sqlite.NewChoresStore(db)
	reStore := sqlite.NewRemindersStore(db)
	flStore := sqlite.NewFlowStore(db)
	notifStore := sqlite.NewNotificationStore(db)
	tgStore := sqlite.NewTelegramStore(db)

	hhSvc := household.NewService(hhStore)
	if err := hhSvc.SyncFromConfig(ctx, cfg.Users, cfg.App.DefaultLocale); err != nil {
		logger.Error("failed to sync household members", "err", err)
		os.Exit(1)
	}

	shSvc := shopping.NewService(shStore, hhSvc)
	chSvc := chores.NewService(chStore, hhSvc, clk)
	reSvc := reminders.NewService(reStore, hhSvc, clk)
	flMgr := flow.NewManager(flStore, clk)

	tgClient := telegram.NewHTTPClient(cfg.BotToken)

	regCtx, regCancel := context.WithTimeout(ctx, 10*time.Second)
	telegram.RegisterCommands(regCtx, tgClient, bundle, cfg.Telegram.ChatID, bundle.Locales(), logger)
	regCancel()

	// Best effort, with its own timeout so a slow command registration cannot starve it:
	// without a username the unconnected notice is sent without its connect button.
	botUsername := ""
	meCtx, meCancel := context.WithTimeout(ctx, 10*time.Second)
	if me, err := tgClient.GetMe(meCtx); err != nil {
		logger.Warn("failed to read bot username, connect button disabled", "err", err)
	} else {
		botUsername = me.Username
	}
	meCancel()

	tgNotifier := telegram.NewNotifier(tgClient, hhSvc, bundle, cfg.Telegram.ChatID, cfg.App.DefaultLocale)

	sched := scheduler.New(scheduler.Config{
		ChoresStore:      chStore,
		RemindersStore:   reStore,
		RemindersService: reSvc,
		NotifStore:       notifStore,
		Notifier:         tgNotifier,
		Onboarding:       hhSvc,
		Clock:            clk,
		ReminderInterval: cfg.App.Notifications.UnresolvedReminderInterval,
		Logger:           logger,
	})

	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := sched.ScanAndNotify(ctx); err != nil && !errors.Is(err, context.Canceled) {
					logger.Error("scheduler scan error", "err", err)
				}
				if _, err := flMgr.Purge(ctx); err != nil && !errors.Is(err, context.Canceled) {
					logger.Error("flow purge error", "err", err)
				}
			}
		}
	}()

	router := telegram.NewRouter(
		telegram.RouterConfig{
			ChatID:           cfg.Telegram.ChatID,
			ShoppingTopicID:  cfg.Telegram.Topics.Shopping,
			ChoresTopicID:    cfg.Telegram.Topics.Chores,
			RemindersTopicID: cfg.Telegram.Topics.Reminders,
			DefaultLocale:    cfg.App.DefaultLocale,
			FlowTTL:          cfg.App.Flow.TTL,
			DefaultTimeOfDay: cfg.App.Calendar.DefaultTimeOfDay,
			BotUsername:      botUsername,
		},
		tgClient,
		hhSvc,
		shSvc,
		chSvc,
		reSvc,
		flMgr,
		bundle,
		clk,
		logger,
	)

	dispatcher := telegram.NewDispatcher(tgClient, tgStore, router, logger)

	logger.Info("homectl service initialized successfully, starting long polling")
	if err := dispatcher.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("dispatcher error", "err", err)
		os.Exit(1)
	}

	logger.Info("homectl service stopped gracefully")
}
