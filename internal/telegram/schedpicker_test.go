package telegram_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/px1/home-ctl/internal/i18n"
	"github.com/px1/home-ctl/internal/telegram"
)

func TestValidateTypedInterval(t *testing.T) {
	tests := []struct {
		input   string
		want    int
		wantErr bool
	}{
		{"1", 1, false},
		{"12", 12, false},
		{"99", 99, false},
		{"07", 7, false},
		{" 15 ", 15, false},
		{"0", 0, true},
		{"100", 0, true},
		{"abc", 0, true},
		{"-5", 0, true},
		{"", 0, true},
		{"1.5", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := telegram.ValidateTypedInterval(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateTypedInterval(%q) error = %v, wantErr = %v", tt.input, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Fatalf("ValidateTypedInterval(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestValidateTypedMinute(t *testing.T) {
	tests := []struct {
		input   string
		want    int
		wantErr bool
	}{
		{"0", 0, false},
		{"00", 0, false},
		{"07", 7, false},
		{"30", 30, false},
		{"59", 59, false},
		{" 45 ", 45, false},
		{"60", 0, true},
		{"75", 0, true},
		{"-1", 0, true},
		{"abc", 0, true},
		{"", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := telegram.ValidateTypedMinute(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateTypedMinute(%q) error = %v, wantErr = %v", tt.input, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Fatalf("ValidateTypedMinute(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestRenderers_EnglishAndSpanish(t *testing.T) {
	bundle, err := i18n.NewBundle(filepath.Join("..", "..", "locales"), "en", i18n.DefaultRequiredKeys, nil)
	if err != nil {
		t.Fatalf("NewBundle failed: %v", err)
	}

	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	// 1. Recurrence type step
	textEN, kbEN := telegram.RenderRecurrenceTypeStep(bundle, "en")
	if !strings.Contains(textEN, "Choose recurrence") {
		t.Errorf("unexpected EN recurrence text: %q", textEN)
	}
	if len(kbEN.InlineKeyboard) != 3 {
		t.Fatalf("expected 3 rows for recurrence step, got %d", len(kbEN.InlineKeyboard))
	}
	if kbEN.InlineKeyboard[0][0].CallbackData != "fl:rt:once" || kbEN.InlineKeyboard[0][1].CallbackData != "fl:rt:daily" {
		t.Errorf("unexpected callbacks in recurrence row 0: %+v", kbEN.InlineKeyboard[0])
	}

	textES, _ := telegram.RenderRecurrenceTypeStep(bundle, "es")
	if !strings.Contains(textES, "Elige") {
		t.Errorf("unexpected ES recurrence text: %q", textES)
	}

	// 2. Interval step
	textIv, kbIv := telegram.RenderIntervalStep(bundle, "en", "weekly")
	if !strings.Contains(textIv, "Repeat every how many weeks?") || strings.Contains(textIv, "(s)") {
		t.Errorf("expected plural unit weeks in interval text, got %q", textIv)
	}
	if len(kbIv.InlineKeyboard) != 4 {
		t.Fatalf("expected 4 rows in interval keyboard, got %d", len(kbIv.InlineKeyboard))
	}
	// Check grid has 1-12
	if kbIv.InlineKeyboard[0][0].Text != "1" || kbIv.InlineKeyboard[2][3].Text != "12" {
		t.Errorf("unexpected interval buttons: 1=%s, 12=%s", kbIv.InlineKeyboard[0][0].Text, kbIv.InlineKeyboard[2][3].Text)
	}
	// Check other button
	if kbIv.InlineKeyboard[3][0].CallbackData != "fl:iv:o" || kbIv.InlineKeyboard[3][0].Text != "Other" {
		t.Errorf("unexpected Other button: %+v", kbIv.InlineKeyboard[3][0])
	}

	// 3. Month calendar step
	// Fixed time: Oct 5, 2026
	now := time.Date(2026, time.October, 5, 12, 0, 0, 0, loc)

	// English calendar: October 2026
	calTextEN, calKbEN := telegram.RenderCalendarStep(bundle, "en", 2026, time.October, now, loc)
	if !strings.Contains(calTextEN, "due date") {
		t.Errorf("unexpected calendar prompt: %q", calTextEN)
	}

	// Row 0 is nav row
	navRow := calKbEN.InlineKeyboard[0]
	if len(navRow) != 3 {
		t.Fatalf("expected 3 buttons in nav row, got %d", len(navRow))
	}
	// Previous button on current month must be fl:noop
	if navRow[0].Text != "◀" || navRow[0].CallbackData != "fl:noop" {
		t.Errorf("expected '<' with 'fl:noop' on current month, got %+v", navRow[0])
	}
	// Title button has English month name
	if navRow[1].Text != "October 2026" || navRow[1].CallbackData != "fl:noop" {
		t.Errorf("expected 'October 2026' title, got %+v", navRow[1])
	}
	// Next button has calendar navigation
	if navRow[2].Text != "▶" || navRow[2].CallbackData != "fl:cal:2026-11" {
		t.Errorf("expected '>' with 'fl:cal:2026-11', got %+v", navRow[2])
	}

	// Row 1 is weekday row (Monday first)
	weekdayRow := calKbEN.InlineKeyboard[1]
	if len(weekdayRow) != 7 {
		t.Fatalf("expected 7 columns in weekday row, got %d", len(weekdayRow))
	}
	expectedWeekdaysEN := []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
	for i, w := range expectedWeekdaysEN {
		if weekdayRow[i].Text != w || weekdayRow[i].CallbackData != "fl:noop" {
			t.Errorf("weekday column %d: got %s (%s), want %s", i, weekdayRow[i].Text, weekdayRow[i].CallbackData, w)
		}
	}

	// Oct 1 2026 is a Thursday (index 3 in Monday-first order: Mon=0, Tue=1, Wed=2, Thu=3)
	// Day row 1 (calKbEN.InlineKeyboard[2])
	dayRow1 := calKbEN.InlineKeyboard[2]
	if len(dayRow1) != 7 {
		t.Fatalf("expected 7 columns in day row 1, got %d", len(dayRow1))
	}
	// Columns 0, 1, 2 must be padding (" ", fl:noop)
	for i := 0; i < 3; i++ {
		if dayRow1[i].Text != " " || dayRow1[i].CallbackData != "fl:noop" {
			t.Errorf("padding col %d: expected ' ' with fl:noop, got %+v", i, dayRow1[i])
		}
	}
	// Column 3 must be Oct 1 under Thursday
	if dayRow1[3].Text != "1" {
		t.Fatalf("expected Oct 1 under Thursday (col 3), got %s", dayRow1[3].Text)
	}

	// Verify Oct 1, 2, 3, 4 are past days (since now is Oct 5) and use fl:noop
	if dayRow1[3].CallbackData != "fl:noop" { // Oct 1
		t.Errorf("Oct 1 is past, expected fl:noop, got %s", dayRow1[3].CallbackData)
	}
	if dayRow1[4].Text != "2" || dayRow1[4].CallbackData != "fl:noop" { // Oct 2
		t.Errorf("Oct 2 is past, expected fl:noop, got %+v", dayRow1[4])
	}
	if dayRow1[5].Text != "3" || dayRow1[5].CallbackData != "fl:noop" { // Oct 3
		t.Errorf("Oct 3 is past, expected fl:noop, got %+v", dayRow1[5])
	}
	if dayRow1[6].Text != "4" || dayRow1[6].CallbackData != "fl:noop" { // Oct 4
		t.Errorf("Oct 4 is past, expected fl:noop, got %+v", dayRow1[6])
	}

	// Oct 5 is today, must be clickable: fl:d:2026-10-05
	dayRow2 := calKbEN.InlineKeyboard[3]
	if dayRow2[0].Text != "5" || dayRow2[0].CallbackData != "fl:d:2026-10-05" {
		t.Errorf("Oct 5 is today, expected fl:d:2026-10-05, got %+v", dayRow2[0])
	}

	// Total day buttons count across day rows
	dayRows := calKbEN.InlineKeyboard[2 : len(calKbEN.InlineKeyboard)-2]
	dayButtonCount := 0
	for _, row := range dayRows {
		if len(row) != 7 {
			t.Errorf("day row has %d columns, want 7", len(row))
		}
		for _, btn := range row {
			if btn.Text != " " {
				dayButtonCount++
			}
		}
	}
	if dayButtonCount != 31 {
		t.Errorf("expected 31 day buttons in October 2026, got %d", dayButtonCount)
	}

	// Shortcut rows
	shortcuts := calKbEN.InlineKeyboard[len(calKbEN.InlineKeyboard)-2]
	if shortcuts[0].Text != "Today" || shortcuts[0].CallbackData != "fl:d:today" {
		t.Errorf("unexpected Today shortcut: %+v", shortcuts[0])
	}
	if shortcuts[1].Text != "Tomorrow" || shortcuts[1].CallbackData != "fl:d:tomorrow" {
		t.Errorf("unexpected Tomorrow shortcut: %+v", shortcuts[1])
	}

	actions := calKbEN.InlineKeyboard[len(calKbEN.InlineKeyboard)-1]
	if actions[0].Text != "Type date" || actions[0].CallbackData != "fl:d:type" {
		t.Errorf("unexpected Type date action: %+v", actions[0])
	}
	if actions[1].Text != "❌ Cancel" || actions[1].CallbackData != "fl:c" {
		t.Errorf("unexpected Cancel action: %+v", actions[1])
	}

	// Spanish calendar: October 2026
	_, calKbES := telegram.RenderCalendarStep(bundle, "es", 2026, time.October, now, loc)
	navRowES := calKbES.InlineKeyboard[0]
	if navRowES[1].Text != "octubre 2026" {
		t.Errorf("expected Spanish header 'octubre 2026', got %q", navRowES[1].Text)
	}

	weekdayRowES := calKbES.InlineKeyboard[1]
	expectedWeekdaysES := []string{"lun", "mar", "mié", "jue", "vie", "sáb", "dom"}
	for i, w := range expectedWeekdaysES {
		if weekdayRowES[i].Text != w {
			t.Errorf("Spanish weekday col %d: got %s, want %s", i, weekdayRowES[i].Text, w)
		}
	}

	shortcutsES := calKbES.InlineKeyboard[len(calKbES.InlineKeyboard)-2]
	if shortcutsES[0].Text != "Hoy" || shortcutsES[1].Text != "Mañana" {
		t.Errorf("expected Hoy and Mañana shortcuts in Spanish, got %s and %s", shortcutsES[0].Text, shortcutsES[1].Text)
	}

	// Future month calendar: November 2026 allows going back to October 2026
	_, calKbNov := telegram.RenderCalendarStep(bundle, "en", 2026, time.November, now, loc)
	if calKbNov.InlineKeyboard[0][0].CallbackData != "fl:cal:2026-10" {
		t.Errorf("expected prev button in Nov 2026 to have 'fl:cal:2026-10', got %s", calKbNov.InlineKeyboard[0][0].CallbackData)
	}

	// 4. Hour step (6 columns by 4 rows)
	textH, kbH := telegram.RenderHourStep(bundle, "en", "2026-10-10", now, loc)
	if !strings.Contains(textH, "Choose hour") {
		t.Errorf("unexpected hour text: %q", textH)
	}
	if len(kbH.InlineKeyboard) != 5 { // 4 hour rows + 1 cancel row
		t.Fatalf("expected 5 rows in hour keyboard, got %d", len(kbH.InlineKeyboard))
	}
	hourCount := 0
	for r := 0; r < 4; r++ {
		row := kbH.InlineKeyboard[r]
		if len(row) != 6 {
			t.Errorf("hour row %d: got %d columns, want 6", r, len(row))
		}
		for _, b := range row {
			hourCount++
			if !strings.HasPrefix(b.CallbackData, "fl:h:") {
				t.Errorf("invalid hour callback: %s", b.CallbackData)
			}
		}
	}
	if hourCount != 24 {
		t.Errorf("expected 24 hour buttons, got %d", hourCount)
	}
	if kbH.InlineKeyboard[0][0].Text != "00" || kbH.InlineKeyboard[3][5].Text != "23" {
		t.Errorf("first hour %s, last hour %s", kbH.InlineKeyboard[0][0].Text, kbH.InlineKeyboard[3][5].Text)
	}

	// 5. Minute step (4 columns by 3 rows + other/cancel row)
	textM, kbM := telegram.RenderMinuteStep(bundle, "en", "2026-10-10", 10, now, loc)
	if !strings.Contains(textM, "Choose minute") {
		t.Errorf("unexpected minute text: %q", textM)
	}
	if len(kbM.InlineKeyboard) != 4 { // 3 minute rows + 1 other/cancel row
		t.Fatalf("expected 4 rows in minute keyboard, got %d", len(kbM.InlineKeyboard))
	}
	minuteCount := 0
	for r := 0; r < 3; r++ {
		row := kbM.InlineKeyboard[r]
		if len(row) != 4 {
			t.Errorf("minute row %d: got %d columns, want 4", r, len(row))
		}
		for _, b := range row {
			minuteCount++
			if !strings.HasPrefix(b.CallbackData, "fl:mi:") {
				t.Errorf("invalid minute callback: %s", b.CallbackData)
			}
		}
	}
	if minuteCount != 12 {
		t.Errorf("expected 12 minute buttons, got %d", minuteCount)
	}
	if kbM.InlineKeyboard[0][0].Text != "00" || kbM.InlineKeyboard[2][3].Text != "55" {
		t.Errorf("first minute %s, last minute %s", kbM.InlineKeyboard[0][0].Text, kbM.InlineKeyboard[2][3].Text)
	}
	if kbM.InlineKeyboard[3][0].CallbackData != "fl:mi:o" {
		t.Errorf("expected Other button with fl:mi:o, got %s", kbM.InlineKeyboard[3][0].CallbackData)
	}

	// 6. Anchor step
	textAnc, kbAnc := telegram.RenderAnchorStep(bundle, "en")
	if !strings.Contains(textAnc, "anchor") {
		t.Errorf("unexpected anchor text: %q", textAnc)
	}
	if len(kbAnc.InlineKeyboard) != 2 {
		t.Fatalf("expected 2 rows in anchor keyboard, got %d", len(kbAnc.InlineKeyboard))
	}
	if kbAnc.InlineKeyboard[0][0].CallbackData != "fl:anc:scheduled" || kbAnc.InlineKeyboard[0][1].CallbackData != "fl:anc:completion" {
		t.Errorf("unexpected anchor callbacks: %+v", kbAnc.InlineKeyboard[0])
	}

	// 7. Confirmation summary step
	due := time.Date(2026, time.October, 10, 10, 30, 0, 0, loc)
	sumTextEN, sumKbEN := telegram.RenderConfirmationStep(bundle, "en", telegram.ConfirmationSummary{
		Emoji:       "🧹",
		Title:       "Change water purifier filter",
		PersonLabel: "picker.summary_assignee",
		PersonName:  "Alice",
		DueAt:       due,
		RecType:     "monthly",
		Interval:    3,
		Anchor:      "completion",
	}, now, loc)
	if !strings.Contains(sumTextEN, "🧹 Change water purifier filter") ||
		!strings.Contains(sumTextEN, "👤 Alice") ||
		!strings.Contains(sumTextEN, "🔁 Every 3 months") ||
		!strings.Contains(sumTextEN, "⏭️ The next one is calculated on completion") {
		t.Errorf("unexpected EN summary text: %q", sumTextEN)
	}
	if len(sumKbEN.InlineKeyboard) != 1 || len(sumKbEN.InlineKeyboard[0]) != 2 {
		t.Fatalf("expected Confirm and Cancel buttons in summary, got %+v", sumKbEN.InlineKeyboard)
	}
	if sumKbEN.InlineKeyboard[0][0].CallbackData != "fl:ok" || sumKbEN.InlineKeyboard[0][1].CallbackData != "fl:c" {
		t.Errorf("unexpected summary callbacks: %+v", sumKbEN.InlineKeyboard[0])
	}

	// One-time summary has no anchor
	sumOnceText, _ := telegram.RenderConfirmationStep(bundle, "en", telegram.ConfirmationSummary{
		Emoji:       "⏰",
		Title:       "Buy medicine",
		PersonLabel: "picker.summary_target",
		PersonName:  "Bob",
		DueAt:       due,
		RecType:     "once",
	}, now, loc)
	if !strings.Contains(sumOnceText, "👤 For: Bob") || !strings.Contains(sumOnceText, "🔁 Once") {
		t.Errorf("unexpected one-time summary, got %q", sumOnceText)
	}
	if strings.Contains(sumOnceText, "⏭️") {
		t.Errorf("one-time item must not display a next-occurrence line, got %q", sumOnceText)
	}
}
