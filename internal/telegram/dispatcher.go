// Package telegram provides the interaction adapter, Telegram Bot API client,
// topic routers, and inline keyboard workflows.
package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// UpdateStore checks and marks processed update IDs for deduplication.
type UpdateStore interface {
	IsUpdateProcessed(ctx context.Context, updateID int64) (bool, error)
	MarkUpdateProcessed(ctx context.Context, updateID int64) error
}

// UpdateHandler processes an incoming update.
type UpdateHandler interface {
	HandleUpdate(ctx context.Context, update Update) error
}

// UpdateHandlerFunc allows using an ordinary function as an UpdateHandler.
type UpdateHandlerFunc func(ctx context.Context, update Update) error

// HandleUpdate calls f(ctx, update).
func (f UpdateHandlerFunc) HandleUpdate(ctx context.Context, update Update) error {
	return f(ctx, update)
}

// Dispatcher coordinates fetching updates, deduplicating them using an UpdateStore,
// and dispatching unhandled updates to an UpdateHandler.
type Dispatcher struct {
	client  Client
	store   UpdateStore
	handler UpdateHandler
	logger  *slog.Logger
}

// NewDispatcher creates a new Dispatcher.
func NewDispatcher(client Client, store UpdateStore, handler UpdateHandler, logger *slog.Logger) *Dispatcher {
	if logger == nil {
		logger = slog.Default()
	}
	return &Dispatcher{
		client:  client,
		store:   store,
		handler: handler,
		logger:  logger,
	}
}

// ProcessUpdate checks whether an update has already been processed, and if not,
// delegates it to the handler and marks it as processed.
func (d *Dispatcher) ProcessUpdate(ctx context.Context, update Update) error {
	if d.store != nil {
		processed, err := d.store.IsUpdateProcessed(ctx, update.UpdateID)
		if err != nil {
			return fmt.Errorf("failed to check processed update: %w", err)
		}
		if processed {
			d.logger.Debug("skipping already processed update", "update_id", update.UpdateID)
			return nil
		}
	}

	if d.handler != nil {
		if err := d.handler.HandleUpdate(ctx, update); err != nil {
			d.logger.Error("error handling update", "update_id", update.UpdateID, "err", err)
		}
	}

	if d.store != nil {
		if err := d.store.MarkUpdateProcessed(ctx, update.UpdateID); err != nil {
			return fmt.Errorf("failed to mark update processed: %w", err)
		}
	}

	return nil
}

// PollOnce fetches a batch of updates from offset, processes each update,
// and returns the next offset to poll.
func (d *Dispatcher) PollOnce(ctx context.Context, offset int64, timeout int) (int64, error) {
	updates, err := d.client.GetUpdates(ctx, offset, timeout)
	if err != nil {
		return offset, err
	}

	nextOffset := offset
	for _, u := range updates {
		if u.UpdateID >= nextOffset {
			nextOffset = u.UpdateID + 1
		}
		if err := d.ProcessUpdate(ctx, u); err != nil {
			d.logger.Error("failed to process update", "update_id", u.UpdateID, "err", err)
		}
	}

	return nextOffset, nil
}

// Start starts long polling in a loop until context cancellation.
func (d *Dispatcher) Start(ctx context.Context) error {
	var offset int64
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		nextOffset, err := d.PollOnce(ctx, offset, 30)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			d.logger.Error("polling error, sleeping", "err", err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(2 * time.Second):
			}
			continue
		}
		offset = nextOffset
	}
}
