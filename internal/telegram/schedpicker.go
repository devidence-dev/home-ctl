package telegram

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	// ErrInvalidInterval indicates typed interval is not an integer in [1, 99].
	ErrInvalidInterval = errors.New("interval must be a whole number from 1 to 99")
	// ErrInvalidMinute indicates typed minute is not an integer in [0, 59].
	ErrInvalidMinute = errors.New("minute must be a whole number from 00 to 59")
)

// ValidateTypedInterval validates a typed interval integer between 1 and 99.
func ValidateTypedInterval(input string) (int, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return 0, ErrInvalidInterval
	}
	for _, r := range trimmed {
		if r < '0' || r > '9' {
			return 0, ErrInvalidInterval
		}
	}
	n, err := strconv.Atoi(trimmed)
	if err != nil || n < 1 || n > 99 {
		return 0, ErrInvalidInterval
	}
	return n, nil
}

// ValidateTypedMinute validates a typed minute integer between 0 and 59.
func ValidateTypedMinute(input string) (int, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return 0, ErrInvalidMinute
	}
	for _, r := range trimmed {
		if r < '0' || r > '9' {
			return 0, ErrInvalidMinute
		}
	}
	m, err := strconv.Atoi(trimmed)
	if err != nil || m < 0 || m > 59 {
		return 0, ErrInvalidMinute
	}
	return m, nil
}

// PickerTranslator defines the localization methods needed by the scheduling picker.
type PickerTranslator interface {
	T(locale, key string, args ...any) string
	FormatDateTime(locale string, t, now time.Time, loc *time.Location) string
}

// RenderRecurrenceTypeStep renders the recurrence type selection prompt and keyboard.
func RenderRecurrenceTypeStep(t PickerTranslator, locale string) (string, *InlineKeyboardMarkup) {
	return t.T(locale, "picker.prompt_recurrence"), &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{
				{Text: t.T(locale, "picker.rec_once"), CallbackData: FlowRecurrenceTypeCallback("once")},
				{Text: t.T(locale, "picker.rec_daily"), CallbackData: FlowRecurrenceTypeCallback("daily")},
			},
			{
				{Text: t.T(locale, "picker.rec_weekly"), CallbackData: FlowRecurrenceTypeCallback("weekly")},
				{Text: t.T(locale, "picker.rec_monthly"), CallbackData: FlowRecurrenceTypeCallback("monthly")},
			},
			{
				{Text: t.T(locale, "buttons.cancel"), CallbackData: "fl:c"},
			},
		},
	}
}

// RenderIntervalStep renders the interval selection grid (1-12 plus Other) and prompt.
func RenderIntervalStep(t PickerTranslator, locale string, recType string) (string, *InlineKeyboardMarkup) {
	var unit string
	switch recType {
	case "daily":
		unit = t.T(locale, "picker.unit_days")
	case "weekly":
		unit = t.T(locale, "picker.unit_weeks")
	case "monthly":
		unit = t.T(locale, "picker.unit_months")
	default:
		unit = ""
	}

	prompt := t.T(locale, "picker.prompt_interval", unit)

	rows := [][]InlineKeyboardButton{
		{
			{Text: "1", CallbackData: FlowIntervalCallback("1")},
			{Text: "2", CallbackData: FlowIntervalCallback("2")},
			{Text: "3", CallbackData: FlowIntervalCallback("3")},
			{Text: "4", CallbackData: FlowIntervalCallback("4")},
		},
		{
			{Text: "5", CallbackData: FlowIntervalCallback("5")},
			{Text: "6", CallbackData: FlowIntervalCallback("6")},
			{Text: "7", CallbackData: FlowIntervalCallback("7")},
			{Text: "8", CallbackData: FlowIntervalCallback("8")},
		},
		{
			{Text: "9", CallbackData: FlowIntervalCallback("9")},
			{Text: "10", CallbackData: FlowIntervalCallback("10")},
			{Text: "11", CallbackData: FlowIntervalCallback("11")},
			{Text: "12", CallbackData: FlowIntervalCallback("12")},
		},
		{
			{Text: t.T(locale, "picker.btn_other"), CallbackData: FlowIntervalCallback("o")},
			{Text: t.T(locale, "buttons.cancel"), CallbackData: "fl:c"},
		},
	}

	return prompt, &InlineKeyboardMarkup{InlineKeyboard: rows}
}

// RenderCalendarStep renders the inline month calendar date picker.
func RenderCalendarStep(t PickerTranslator, locale string, year int, month time.Month, now time.Time, loc *time.Location) (string, *InlineKeyboardMarkup) {
	prompt := t.T(locale, "picker.prompt_date")

	nowInLoc := now.In(loc)
	curYear, curMonth, curDay := nowInLoc.Date()
	todayDate := time.Date(curYear, curMonth, curDay, 0, 0, 0, 0, loc)

	// Localized month name
	monthsStr := t.T(locale, "picker.months")
	monthNames := strings.Split(monthsStr, ",")
	var monthName string
	if int(month) >= 1 && int(month) <= len(monthNames) {
		monthName = strings.TrimSpace(monthNames[month-1])
	} else {
		monthName = month.String()
	}
	headerTitle := fmt.Sprintf("%s %d", monthName, year)

	// Navigation buttons
	isCurrentOrPastMonth := (year < curYear) || (year == curYear && month <= curMonth)
	var prevBtn InlineKeyboardButton
	if isCurrentOrPastMonth {
		prevBtn = InlineKeyboardButton{Text: "◀", CallbackData: "fl:noop"}
	} else {
		prevYear := year
		prevMonth := int(month) - 1
		if prevMonth < 1 {
			prevYear--
			prevMonth = 12
		}
		prevBtn = InlineKeyboardButton{
			Text:         "◀",
			CallbackData: FlowCalendarCallback(fmt.Sprintf("%04d-%02d", prevYear, prevMonth)),
		}
	}

	titleBtn := InlineKeyboardButton{Text: headerTitle, CallbackData: "fl:noop"}

	nextYear := year
	nextMonth := int(month) + 1
	if nextMonth > 12 {
		nextYear++
		nextMonth = 1
	}
	nextBtn := InlineKeyboardButton{
		Text:         "▶",
		CallbackData: FlowCalendarCallback(fmt.Sprintf("%04d-%02d", nextYear, nextMonth)),
	}

	navRow := []InlineKeyboardButton{prevBtn, titleBtn, nextBtn}

	// Weekdays row, Monday first
	weekdaysStr := t.T(locale, "datetime.weekdays")
	rawWeekdays := strings.Split(weekdaysStr, ",")
	mondayFirstIndices := []int{1, 2, 3, 4, 5, 6, 0}
	var weekdayRow []InlineKeyboardButton
	for _, idx := range mondayFirstIndices {
		name := ""
		if idx < len(rawWeekdays) {
			name = strings.TrimSpace(rawWeekdays[idx])
		}
		weekdayRow = append(weekdayRow, InlineKeyboardButton{
			Text:         name,
			CallbackData: "fl:noop",
		})
	}

	// Day buttons
	totalDays := time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
	firstDay := time.Date(year, month, 1, 0, 0, 0, 0, loc)
	firstWeekday := firstDay.Weekday()
	var firstOffset int
	if firstWeekday == time.Sunday {
		firstOffset = 6
	} else {
		firstOffset = int(firstWeekday) - 1
	}

	var dayRows [][]InlineKeyboardButton
	var currentRow []InlineKeyboardButton

	for i := 0; i < firstOffset; i++ {
		currentRow = append(currentRow, InlineKeyboardButton{Text: " ", CallbackData: "fl:noop"})
	}

	for d := 1; d <= totalDays; d++ {
		dayDate := time.Date(year, month, d, 0, 0, 0, 0, loc)
		isPast := dayDate.Before(todayDate)

		var cb string
		if isPast {
			cb = "fl:noop"
		} else {
			cb = FlowDateCallback(fmt.Sprintf("%04d-%02d-%02d", year, int(month), d))
		}

		currentRow = append(currentRow, InlineKeyboardButton{
			Text:         strconv.Itoa(d),
			CallbackData: cb,
		})

		if len(currentRow) == 7 {
			dayRows = append(dayRows, currentRow)
			currentRow = nil
		}
	}

	if len(currentRow) > 0 {
		for len(currentRow) < 7 {
			currentRow = append(currentRow, InlineKeyboardButton{Text: " ", CallbackData: "fl:noop"})
		}
		dayRows = append(dayRows, currentRow)
	}

	shortcutRow := []InlineKeyboardButton{
		{Text: t.T(locale, "picker.btn_today"), CallbackData: FlowDateCallback("today")},
		{Text: t.T(locale, "picker.btn_tomorrow"), CallbackData: FlowDateCallback("tomorrow")},
	}

	actionRow := []InlineKeyboardButton{
		{Text: t.T(locale, "picker.btn_type_date"), CallbackData: FlowDateCallback("type")},
		{Text: t.T(locale, "buttons.cancel"), CallbackData: "fl:c"},
	}

	allRows := make([][]InlineKeyboardButton, 0, 4+len(dayRows))
	allRows = append(allRows, navRow, weekdayRow)
	allRows = append(allRows, dayRows...)
	allRows = append(allRows, shortcutRow, actionRow)

	return prompt, &InlineKeyboardMarkup{InlineKeyboard: allRows}
}

// RenderHourStep renders the 24-hour grid (6 columns by 4 rows).
func RenderHourStep(t PickerTranslator, locale string) (string, *InlineKeyboardMarkup) {
	prompt := t.T(locale, "picker.prompt_hour")

	rows := make([][]InlineKeyboardButton, 0, 5)
	for r := 0; r < 4; r++ {
		row := make([]InlineKeyboardButton, 0, 6)
		for c := 0; c < 6; c++ {
			h := r*6 + c
			row = append(row, InlineKeyboardButton{
				Text:         fmt.Sprintf("%02d", h),
				CallbackData: FlowHourCallback(h),
			})
		}
		rows = append(rows, row)
	}

	rows = append(rows, []InlineKeyboardButton{
		{Text: t.T(locale, "buttons.cancel"), CallbackData: "fl:c"},
	})

	return prompt, &InlineKeyboardMarkup{InlineKeyboard: rows}
}

// RenderMinuteStep renders the 5-minute grid (4 columns by 3 rows) plus Other and Cancel.
func RenderMinuteStep(t PickerTranslator, locale string) (string, *InlineKeyboardMarkup) {
	prompt := t.T(locale, "picker.prompt_minute")

	rows := make([][]InlineKeyboardButton, 0, 4)
	for r := 0; r < 3; r++ {
		row := make([]InlineKeyboardButton, 0, 4)
		for c := 0; c < 4; c++ {
			m := (r*4 + c) * 5
			row = append(row, InlineKeyboardButton{
				Text:         fmt.Sprintf("%02d", m),
				CallbackData: FlowMinuteCallback(m),
			})
		}
		rows = append(rows, row)
	}

	rows = append(rows, []InlineKeyboardButton{
		{Text: t.T(locale, "picker.btn_other"), CallbackData: "fl:mi:o"},
		{Text: t.T(locale, "buttons.cancel"), CallbackData: "fl:c"},
	})

	return prompt, &InlineKeyboardMarkup{InlineKeyboard: rows}
}

// RenderAnchorStep renders the recurrence anchor choice prompt and keyboard.
func RenderAnchorStep(t PickerTranslator, locale string) (string, *InlineKeyboardMarkup) {
	prompt := t.T(locale, "picker.prompt_anchor")

	rows := [][]InlineKeyboardButton{
		{
			{Text: t.T(locale, "picker.anchor_scheduled"), CallbackData: "fl:anc:scheduled"},
			{Text: t.T(locale, "picker.anchor_completion"), CallbackData: "fl:anc:completion"},
		},
		{
			{Text: t.T(locale, "buttons.cancel"), CallbackData: "fl:c"},
		},
	}

	return prompt, &InlineKeyboardMarkup{InlineKeyboard: rows}
}

// RenderConfirmationStep renders the confirmation summary and confirm/cancel buttons.
func RenderConfirmationStep(
	t PickerTranslator,
	locale string,
	title string,
	personLabel string,
	personName string,
	dueTime time.Time,
	recType string,
	interval int,
	anchor string,
	now time.Time,
	loc *time.Location,
) (string, *InlineKeyboardMarkup) {
	var recDesc string
	switch recType {
	case "daily":
		recDesc = t.T(locale, "general.recurrence_every_day", interval)
	case "weekly":
		recDesc = t.T(locale, "general.recurrence_every_week", interval)
	case "monthly":
		recDesc = t.T(locale, "general.recurrence_every_month", interval)
	default:
		recDesc = t.T(locale, "general.recurrence_one_off")
	}

	dueFormatted := t.FormatDateTime(locale, dueTime, now, loc)

	var sb strings.Builder
	sb.WriteString(t.T(locale, "picker.prompt_confirm"))
	sb.WriteString("\n\n")
	fmt.Fprintf(&sb, "%s: %s\n", t.T(locale, "picker.summary_title"), title)
	fmt.Fprintf(&sb, "%s: %s\n", personLabel, personName)
	fmt.Fprintf(&sb, "%s: %s\n", t.T(locale, "picker.summary_due"), dueFormatted)
	fmt.Fprintf(&sb, "%s: %s", t.T(locale, "picker.summary_recurrence"), recDesc)

	if recType != "once" && recType != "" {
		var ancDesc string
		if anchor == "completion" {
			ancDesc = t.T(locale, "picker.anchor_completion")
		} else {
			ancDesc = t.T(locale, "picker.anchor_scheduled")
		}
		fmt.Fprintf(&sb, "\n%s: %s", t.T(locale, "picker.summary_anchor"), ancDesc)
	}

	rows := [][]InlineKeyboardButton{
		{
			{Text: t.T(locale, "buttons.confirm"), CallbackData: "fl:ok"},
			{Text: t.T(locale, "buttons.cancel"), CallbackData: "fl:c"},
		},
	}

	return sb.String(), &InlineKeyboardMarkup{InlineKeyboard: rows}
}
