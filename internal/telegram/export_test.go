package telegram

import (
	"context"

	"github.com/px1/home-ctl/internal/flow"
)

// ShowFlowMessageForTest exposes showFlowMessage to package telegram_test.
func (r *Router) ShowFlowMessageForTest(ctx context.Context, state *flow.State, text string, markup *InlineKeyboardMarkup, forceSend ...bool) error {
	return r.showFlowMessage(ctx, state, text, markup, forceSend...)
}
