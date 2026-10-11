package telegram

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	// ErrInvalidInterval indicates typed interval is not an integer in [1, 99].
	ErrInvalidInterval = errors.New("interval must be a whole number from 1 to 99")
	// ErrInvalidMinute indicates typed minute is not an integer in [0, 59].
	ErrInvalidMinute = errors.New("minute must be a whole number from 00 to 59")
	// ErrInvalidEmoji indicates typed emoji is empty, too long, or contains letters or digits.
	ErrInvalidEmoji = errors.New("emoji must be a short symbol without letters or digits")
)

// maxTypedEmojiRunes bounds a typed emoji; flags, skin tones and ZWJ sequences fit well within it.
const maxTypedEmojiRunes = 16

// ValidateTypedEmoji returns the trimmed emoji typed by the member. Plain text, digits and empty input are rejected.
func ValidateTypedEmoji(input string) (string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" || utf8.RuneCountInString(trimmed) > maxTypedEmojiRunes {
		return "", ErrInvalidEmoji
	}
	for _, r := range trimmed {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return "", ErrInvalidEmoji
		}
	}
	return trimmed, nil
}

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
	// One whole question per unit: Spanish needs "cuántos días" but "cuántas semanas".
	key := "picker.prompt_interval_days"
	switch recType {
	case "weekly":
		key = "picker.prompt_interval_weeks"
	case "monthly":
		key = "picker.prompt_interval_months"
	}
	prompt := t.T(locale, key)

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

// PickerContext holds the values already chosen in a scheduling flow; they head every step after the title.
type PickerContext struct {
	Emoji  string
	Title  string
	Person string
	Date   string // YYYY-MM-DD in the household timezone, empty until chosen
}

// RenderContextHeader renders the chosen values on one line ("🧹 Clean kitchen · Alice · October 14").
// It returns "" while no title is set.
func RenderContextHeader(t PickerTranslator, locale string, pc PickerContext, now time.Time, loc *time.Location) string {
	if pc.Title == "" {
		return ""
	}
	parts := []string{strings.TrimSpace(pc.Emoji + " " + pc.Title)}
	if pc.Person != "" {
		parts = append(parts, pc.Person)
	}
	if pc.Date != "" {
		if day, err := time.ParseInLocation("2006-01-02", pc.Date, loc); err == nil {
			monthName := day.Month().String()
			if months := strings.Split(t.T(locale, "picker.months"), ","); int(day.Month()) <= len(months) {
				monthName = strings.TrimSpace(months[day.Month()-1])
			}
			date := fmt.Sprintf("%s %d", monthName, day.Day())
			if day.Year() != now.In(loc).Year() {
				date += fmt.Sprintf(" %d", day.Year())
			}
			parts = append(parts, date)
		}
	}
	return strings.Join(parts, " · ")
}

// RecurrenceText renders "Every week" or "Every 2 weeks" from the singular or plural locale form.
// unit is "day", "week" or "month".
func RecurrenceText(t PickerTranslator, locale string, interval int, unit string) string {
	if interval == 1 {
		return t.T(locale, "general.recurrence_every_"+unit+"_one")
	}
	return t.T(locale, "general.recurrence_every_"+unit+"_other", interval)
}

// isToday reports whether dueDate (YYYY-MM-DD) is the current day in loc.
func isToday(dueDate string, now time.Time, loc *time.Location) bool {
	return dueDate != "" && now.In(loc).Format("2006-01-02") == dueDate
}

// RenderHourStep renders the 24-hour grid (6 columns by 4 rows) with Change date.
// For today, hours before the current hour are inert.
func RenderHourStep(t PickerTranslator, locale string, dueDate string, now time.Time, loc *time.Location) (string, *InlineKeyboardMarkup) {
	prompt := t.T(locale, "picker.prompt_hour")
	today := isToday(dueDate, now, loc)
	nowHour := now.In(loc).Hour()

	rows := make([][]InlineKeyboardButton, 0, 6)
	for r := 0; r < 4; r++ {
		row := make([]InlineKeyboardButton, 0, 6)
		for c := 0; c < 6; c++ {
			h := r*6 + c
			btn := InlineKeyboardButton{Text: fmt.Sprintf("%02d", h), CallbackData: FlowHourCallback(h)}
			if today && h < nowHour {
				btn = InlineKeyboardButton{Text: "·", CallbackData: "fl:noop"}
			}
			row = append(row, btn)
		}
		rows = append(rows, row)
	}

	rows = append(rows, []InlineKeyboardButton{
		{Text: t.T(locale, "picker.btn_change_date"), CallbackData: "fl:d:back"},
		{Text: t.T(locale, "buttons.cancel"), CallbackData: "fl:c"},
	})

	return prompt, &InlineKeyboardMarkup{InlineKeyboard: rows}
}

// RenderMinuteStep renders the 5-minute grid (4 columns by 3 rows) plus Other and Cancel.
// For today's current hour, minutes not later than the current minute are inert.
func RenderMinuteStep(t PickerTranslator, locale string, dueDate string, hour int, now time.Time, loc *time.Location) (string, *InlineKeyboardMarkup) {
	prompt := t.T(locale, "picker.prompt_minute")
	local := now.In(loc)
	today := isToday(dueDate, now, loc)

	rows := make([][]InlineKeyboardButton, 0, 4)
	for r := 0; r < 3; r++ {
		row := make([]InlineKeyboardButton, 0, 4)
		for c := 0; c < 4; c++ {
			m := (r*4 + c) * 5
			btn := InlineKeyboardButton{Text: fmt.Sprintf("%02d", m), CallbackData: FlowMinuteCallback(m)}
			if today && (hour < local.Hour() || (hour == local.Hour() && m <= local.Minute())) {
				btn = InlineKeyboardButton{Text: "·", CallbackData: "fl:noop"}
			}
			row = append(row, btn)
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

// EmojiSuggestions returns the comma-separated suggestion list stored under key in the locale file.
func EmojiSuggestions(t PickerTranslator, locale string, key string) []string {
	return strings.Split(t.T(locale, key), ",")
}

// RenderEmojiStep renders the emoji suggestions in rows of four, then Other, Skip and Cancel.
// Suggestion buttons carry only their index, which the handler resolves against the same list.
func RenderEmojiStep(t PickerTranslator, locale string, suggestions []string) (string, *InlineKeyboardMarkup) {
	prompt := t.T(locale, "picker.prompt_emoji")

	var rows [][]InlineKeyboardButton
	for i, s := range suggestions {
		if i%4 == 0 {
			rows = append(rows, []InlineKeyboardButton{})
		}
		last := len(rows) - 1
		rows[last] = append(rows[last], InlineKeyboardButton{Text: s, CallbackData: fmt.Sprintf("fl:em:%d", i)})
	}
	rows = append(rows, []InlineKeyboardButton{
		{Text: t.T(locale, "picker.btn_other"), CallbackData: "fl:em:o"},
		{Text: t.T(locale, "picker.btn_skip"), CallbackData: "fl:em:skip"},
		{Text: t.T(locale, "buttons.cancel"), CallbackData: "fl:c"},
	})

	return prompt, &InlineKeyboardMarkup{InlineKeyboard: rows}
}

// ConfirmationSummary holds the values shown on the final confirmation step.
type ConfirmationSummary struct {
	Emoji       string
	Title       string
	PersonLabel string // locale key of the person line: picker.summary_assignee or picker.summary_target
	PersonName  string
	DueAt       time.Time
	RecType     string // once, daily, weekly or monthly
	Interval    int
	Anchor      string // scheduled or completion; used only for recurring items
}

// RenderConfirmationStep renders one emoji line per value and the Confirm and Cancel buttons.
func RenderConfirmationStep(t PickerTranslator, locale string, s ConfirmationSummary, now time.Time, loc *time.Location) (string, *InlineKeyboardMarkup) {
	var sb strings.Builder
	sb.WriteString(t.T(locale, "picker.prompt_confirm"))
	sb.WriteString("\n\n")
	fmt.Fprintf(&sb, "%s %s\n", s.Emoji, s.Title)
	fmt.Fprintf(&sb, "%s\n", t.T(locale, s.PersonLabel, s.PersonName))
	fmt.Fprintf(&sb, "%s\n", t.T(locale, "picker.summary_due", t.FormatDateTime(locale, s.DueAt, now, loc)))

	if unit := recurrenceUnit(s.RecType); unit != "" {
		fmt.Fprintf(&sb, "%s\n", t.T(locale, "picker.summary_recurrence", RecurrenceText(t, locale, s.Interval, unit)))
		anchorKey := "picker.summary_anchor_scheduled"
		if s.Anchor == "completion" {
			anchorKey = "picker.summary_anchor_completion"
		}
		sb.WriteString(t.T(locale, anchorKey))
	} else {
		sb.WriteString(t.T(locale, "picker.summary_once"))
	}

	rows := [][]InlineKeyboardButton{
		{
			{Text: t.T(locale, "buttons.confirm"), CallbackData: "fl:ok"},
			{Text: t.T(locale, "buttons.cancel"), CallbackData: "fl:c"},
		},
	}

	return sb.String(), &InlineKeyboardMarkup{InlineKeyboard: rows}
}

// recurrenceUnit maps a picker recurrence type to its locale unit name, or "" for one-time items.
func recurrenceUnit(recType string) string {
	switch recType {
	case "daily":
		return "day"
	case "weekly":
		return "week"
	case "monthly":
		return "month"
	}
	return ""
}
