package telegram

import (
	"errors"
	"fmt"
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
	CallbackShoppingPurchase   CallbackKind = "sh:p"
	CallbackChoreComplete      CallbackKind = "ch:c"
	CallbackChorePostpone      CallbackKind = "ch:p"
	CallbackChoreDetail        CallbackKind = "ch:d"
	CallbackReminderComplete   CallbackKind = "re:c"
	CallbackReminderPostpone   CallbackKind = "re:p"
	CallbackReminderDetail     CallbackKind = "re:d"
	CallbackListChores         CallbackKind = "list:ch"
	CallbackListReminders      CallbackKind = "list:re"
	CallbackFlowCancel         CallbackKind = "fl:c"
	CallbackFlowConfirm        CallbackKind = "fl:ok"
	CallbackFlowSelect         CallbackKind = "fl:sel"
	CallbackFlowUser           CallbackKind = "fl:usr"
	CallbackFlowDue            CallbackKind = "fl:due"
	CallbackFlowRecurrence     CallbackKind = "fl:rec"
	CallbackFlowAnchor         CallbackKind = "fl:anc"
	CallbackFlowNewItem        CallbackKind = "fl:new"
	CallbackFlowNoop           CallbackKind = "fl:noop"
	CallbackFlowRecurrenceType CallbackKind = "fl:rt"
	CallbackFlowInterval       CallbackKind = "fl:iv"
	CallbackFlowCalendar       CallbackKind = "fl:cal"
	CallbackFlowDate           CallbackKind = "fl:d"
	CallbackFlowHour           CallbackKind = "fl:h"
	CallbackFlowMinute         CallbackKind = "fl:mi"
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
	case "fl:noop":
		return &ParsedCallback{Kind: CallbackFlowNoop, ActorID: actorID}, nil
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

	case CallbackFlowRecurrenceType:
		if len(parts) != 3 {
			return nil, ErrMalformedCallback
		}
		rt := parts[2]
		if rt != "daily" && rt != "weekly" && rt != "monthly" && rt != "once" {
			return nil, ErrMalformedCallback
		}
		return &ParsedCallback{Kind: CallbackFlowRecurrenceType, Payload: rt, ActorID: actorID}, nil

	case CallbackFlowInterval:
		if len(parts) != 3 {
			return nil, ErrMalformedCallback
		}
		if parts[2] == "o" {
			return &ParsedCallback{Kind: CallbackFlowInterval, Payload: "o", ActorID: actorID}, nil
		}
		n, err := strconv.Atoi(parts[2])
		if err != nil || n < 1 || n > 99 {
			return nil, ErrMalformedCallback
		}
		return &ParsedCallback{Kind: CallbackFlowInterval, TargetID: int64(n), Payload: parts[2], ActorID: actorID}, nil

	case CallbackFlowCalendar:
		if len(parts) != 3 {
			return nil, ErrMalformedCallback
		}
		calParts := strings.Split(parts[2], "-")
		if len(calParts) != 2 || len(calParts[0]) != 4 || len(calParts[1]) != 2 {
			return nil, ErrMalformedCallback
		}
		y, err1 := strconv.Atoi(calParts[0])
		m, err2 := strconv.Atoi(calParts[1])
		if err1 != nil || err2 != nil || y < 2000 || y > 2100 || m < 1 || m > 12 {
			return nil, ErrMalformedCallback
		}
		return &ParsedCallback{Kind: CallbackFlowCalendar, Payload: parts[2], ActorID: actorID}, nil

	case CallbackFlowDate:
		if len(parts) != 3 {
			return nil, ErrMalformedCallback
		}
		d := parts[2]
		if d == "today" || d == "tomorrow" || d == "type" {
			return &ParsedCallback{Kind: CallbackFlowDate, Payload: d, ActorID: actorID}, nil
		}
		dParts := strings.Split(d, "-")
		if len(dParts) != 3 || len(dParts[0]) != 4 || len(dParts[1]) != 2 || len(dParts[2]) != 2 {
			return nil, ErrMalformedCallback
		}
		y, err1 := strconv.Atoi(dParts[0])
		m, err2 := strconv.Atoi(dParts[1])
		day, err3 := strconv.Atoi(dParts[2])
		if err1 != nil || err2 != nil || err3 != nil || y < 2000 || y > 2100 || m < 1 || m > 12 || day < 1 || day > 31 {
			return nil, ErrMalformedCallback
		}
		return &ParsedCallback{Kind: CallbackFlowDate, Payload: d, ActorID: actorID}, nil

	case CallbackFlowHour:
		if len(parts) != 3 || len(parts[2]) != 2 {
			return nil, ErrMalformedCallback
		}
		h, err := strconv.Atoi(parts[2])
		if err != nil || h < 0 || h > 23 {
			return nil, ErrMalformedCallback
		}
		return &ParsedCallback{Kind: CallbackFlowHour, TargetID: int64(h), Payload: parts[2], ActorID: actorID}, nil

	case CallbackFlowMinute:
		if len(parts) != 3 {
			return nil, ErrMalformedCallback
		}
		if parts[2] == "o" {
			return &ParsedCallback{Kind: CallbackFlowMinute, Payload: "o", ActorID: actorID}, nil
		}
		if len(parts[2]) != 2 {
			return nil, ErrMalformedCallback
		}
		mi, err := strconv.Atoi(parts[2])
		if err != nil || mi < 0 || mi > 59 {
			return nil, ErrMalformedCallback
		}
		return &ParsedCallback{Kind: CallbackFlowMinute, TargetID: int64(mi), Payload: parts[2], ActorID: actorID}, nil

	default:
		return nil, ErrMalformedCallback
	}
}

// FlowRecurrenceTypeCallback returns callback data for a recurrence type.
func FlowRecurrenceTypeCallback(rt string) string {
	return "fl:rt:" + rt
}

// FlowIntervalCallback returns callback data for an interval.
func FlowIntervalCallback(iv string) string {
	return "fl:iv:" + iv
}

// FlowCalendarCallback returns callback data for calendar navigation.
func FlowCalendarCallback(yearMonth string) string {
	return "fl:cal:" + yearMonth
}

// FlowDateCallback returns callback data for date selection.
func FlowDateCallback(dateStr string) string {
	return "fl:d:" + dateStr
}

// FlowHourCallback returns callback data for hour selection.
func FlowHourCallback(hour int) string {
	return fmt.Sprintf("fl:h:%02d", hour)
}

// FlowMinuteCallback returns callback data for minute selection.
func FlowMinuteCallback(minute int) string {
	return fmt.Sprintf("fl:mi:%02d", minute)
}
