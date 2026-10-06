package telegram

import (
	"errors"
	"strconv"
	"strings"
)

var (
	// ErrMalformedCallback is returned when callback data is malformed or invalid.
	ErrMalformedCallback = errors.New("malformed callback data")
)

// CallbackKind represents the category and action of a callback query.
type CallbackKind string

// Callback kinds for routing Telegram inline keyboard interactions.
const (
	CallbackShoppingPurchase CallbackKind = "sh:p"
	CallbackChoreComplete    CallbackKind = "ch:c"
	CallbackChorePostpone    CallbackKind = "ch:p"
	CallbackChoreDetail      CallbackKind = "ch:d"
	CallbackReminderComplete CallbackKind = "re:c"
	CallbackReminderPostpone CallbackKind = "re:p"
	CallbackReminderDetail   CallbackKind = "re:d"
	CallbackListChores       CallbackKind = "list:ch"
	CallbackListReminders    CallbackKind = "list:re"
	CallbackFlowCancel       CallbackKind = "fl:c"
	CallbackFlowConfirm      CallbackKind = "fl:ok"
	CallbackFlowSelect       CallbackKind = "fl:sel"
	CallbackFlowUser         CallbackKind = "fl:usr"
	CallbackFlowDue          CallbackKind = "fl:due"
	CallbackFlowRecurrence   CallbackKind = "fl:rec"
	CallbackFlowAnchor       CallbackKind = "fl:anc"
	CallbackFlowNewItem      CallbackKind = "fl:new"
)

// ParsedCallback holds parsed callback data with the actor guaranteed to be the update sender.
type ParsedCallback struct {
	Kind     CallbackKind
	TargetID int64
	Payload  string
	ActorID  int64 // Sourced strictly from Telegram update From.ID
}

// ParseCallbackData parses a compact Telegram callback_data string.
func ParseCallbackData(data string, actorID int64) (*ParsedCallback, error) {
	if len(data) == 0 || len(data) > 64 {
		return nil, ErrMalformedCallback
	}

	// Exact string matches
	switch data {
	case "list:ch":
		return &ParsedCallback{Kind: CallbackListChores, ActorID: actorID}, nil
	case "list:re":
		return &ParsedCallback{Kind: CallbackListReminders, ActorID: actorID}, nil
	case "fl:c":
		return &ParsedCallback{Kind: CallbackFlowCancel, ActorID: actorID}, nil
	case "fl:ok":
		return &ParsedCallback{Kind: CallbackFlowConfirm, ActorID: actorID}, nil
	case "fl:new":
		return &ParsedCallback{Kind: CallbackFlowNewItem, ActorID: actorID}, nil
	}

	parts := strings.Split(data, ":")
	if len(parts) < 2 {
		return nil, ErrMalformedCallback
	}

	prefix := parts[0] + ":" + parts[1]
	switch CallbackKind(prefix) {
	case CallbackShoppingPurchase:
		if len(parts) != 3 {
			return nil, ErrMalformedCallback
		}
		id, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			return nil, ErrMalformedCallback
		}
		return &ParsedCallback{Kind: CallbackShoppingPurchase, TargetID: id, ActorID: actorID}, nil

	case CallbackChoreComplete:
		if len(parts) != 3 {
			return nil, ErrMalformedCallback
		}
		id, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			return nil, ErrMalformedCallback
		}
		return &ParsedCallback{Kind: CallbackChoreComplete, TargetID: id, ActorID: actorID}, nil

	case CallbackChoreDetail:
		if len(parts) != 3 {
			return nil, ErrMalformedCallback
		}
		id, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			return nil, ErrMalformedCallback
		}
		return &ParsedCallback{Kind: CallbackChoreDetail, TargetID: id, ActorID: actorID}, nil

	case CallbackChorePostpone:
		if len(parts) == 3 {
			id, err := strconv.ParseInt(parts[2], 10, 64)
			if err != nil {
				return nil, ErrMalformedCallback
			}
			return &ParsedCallback{Kind: CallbackChorePostpone, TargetID: id, ActorID: actorID}, nil
		}
		if len(parts) == 4 {
			id, err := strconv.ParseInt(parts[2], 10, 64)
			if err != nil {
				return nil, ErrMalformedCallback
			}
			return &ParsedCallback{Kind: CallbackChorePostpone, TargetID: id, Payload: parts[3], ActorID: actorID}, nil
		}
		return nil, ErrMalformedCallback

	case CallbackReminderComplete:
		if len(parts) != 3 {
			return nil, ErrMalformedCallback
		}
		id, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			return nil, ErrMalformedCallback
		}
		return &ParsedCallback{Kind: CallbackReminderComplete, TargetID: id, ActorID: actorID}, nil

	case CallbackReminderDetail:
		if len(parts) != 3 {
			return nil, ErrMalformedCallback
		}
		id, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			return nil, ErrMalformedCallback
		}
		return &ParsedCallback{Kind: CallbackReminderDetail, TargetID: id, ActorID: actorID}, nil

	case CallbackReminderPostpone:
		if len(parts) == 3 {
			id, err := strconv.ParseInt(parts[2], 10, 64)
			if err != nil {
				return nil, ErrMalformedCallback
			}
			return &ParsedCallback{Kind: CallbackReminderPostpone, TargetID: id, ActorID: actorID}, nil
		}
		if len(parts) == 4 {
			id, err := strconv.ParseInt(parts[2], 10, 64)
			if err != nil {
				return nil, ErrMalformedCallback
			}
			return &ParsedCallback{Kind: CallbackReminderPostpone, TargetID: id, Payload: parts[3], ActorID: actorID}, nil
		}
		return nil, ErrMalformedCallback

	case CallbackFlowSelect:
		if len(parts) != 3 {
			return nil, ErrMalformedCallback
		}
		id, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			return nil, ErrMalformedCallback
		}
		return &ParsedCallback{Kind: CallbackFlowSelect, TargetID: id, ActorID: actorID}, nil

	case CallbackFlowUser:
		if len(parts) != 3 {
			return nil, ErrMalformedCallback
		}
		id, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			return nil, ErrMalformedCallback
		}
		return &ParsedCallback{Kind: CallbackFlowUser, TargetID: id, ActorID: actorID}, nil

	case CallbackFlowDue:
		if len(parts) != 3 {
			return nil, ErrMalformedCallback
		}
		return &ParsedCallback{Kind: CallbackFlowDue, Payload: parts[2], ActorID: actorID}, nil

	case CallbackFlowRecurrence:
		if len(parts) != 3 {
			return nil, ErrMalformedCallback
		}
		return &ParsedCallback{Kind: CallbackFlowRecurrence, Payload: parts[2], ActorID: actorID}, nil

	case CallbackFlowAnchor:
		if len(parts) != 3 {
			return nil, ErrMalformedCallback
		}
		return &ParsedCallback{Kind: CallbackFlowAnchor, Payload: parts[2], ActorID: actorID}, nil

	default:
		return nil, ErrMalformedCallback
	}
}
