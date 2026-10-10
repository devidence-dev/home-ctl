package telegram_test

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/px1/home-ctl/internal/i18n"
	"github.com/px1/home-ctl/internal/telegram"
)

func TestValidateTypedEmoji(t *testing.T) {
	cases := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"🪴", "🪴", false},
		{"  🪴  ", "🪴", false},
		{"🇪🇨", "🇪🇨", false},
		{"👍🏽", "👍🏽", false},
		{"plants", "", true},
		{"7", "", true},
		{"🪴 a", "", true},
		{"", "", true},
		{"   ", "", true},
		{strings.Repeat("🪴", 17), "", true},
	}
	for _, c := range cases {
		got, err := telegram.ValidateTypedEmoji(c.input)
		if c.wantErr {
			if err == nil {
				t.Errorf("ValidateTypedEmoji(%q) accepted, want rejected", c.input)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("ValidateTypedEmoji(%q) = %q, %v; want %q", c.input, got, err, c.want)
		}
	}
}

func TestRenderEmojiStep_Layout(t *testing.T) {
	bundle, err := i18n.NewBundle(filepath.Join("..", "..", "locales"), "en", i18n.DefaultRequiredKeys, nil)
	if err != nil {
		t.Fatalf("NewBundle failed: %v", err)
	}
	suggestions := telegram.EmojiSuggestions(bundle, "en", "picker.emoji_suggestions_chores")
	if len(suggestions) != 8 {
		t.Fatalf("expected 8 chore suggestions, got %d", len(suggestions))
	}

	text, kb := telegram.RenderEmojiStep(bundle, "en", suggestions)
	if text != bundle.T("en", "picker.prompt_emoji") {
		t.Errorf("unexpected prompt %q", text)
	}
	if len(kb.InlineKeyboard) != 3 {
		t.Fatalf("expected two suggestion rows and one control row, got %d rows", len(kb.InlineKeyboard))
	}
	for r := 0; r < 2; r++ {
		if len(kb.InlineKeyboard[r]) != 4 {
			t.Fatalf("row %d: expected 4 suggestions, got %d", r, len(kb.InlineKeyboard[r]))
		}
	}
	if got := kb.InlineKeyboard[1][3]; got.Text != "🌱" || got.CallbackData != "fl:em:7" {
		t.Errorf("last suggestion: got %+v", got)
	}
	controls := kb.InlineKeyboard[2]
	wantControls := []string{"fl:em:o", "fl:em:skip", "fl:c"}
	if len(controls) != len(wantControls) {
		t.Fatalf("expected Other, Skip and Cancel, got %+v", controls)
	}
	for i, want := range wantControls {
		if controls[i].CallbackData != want {
			t.Errorf("control %d: want %q, got %q", i, want, controls[i].CallbackData)
		}
	}

	// Reminder suggestions come from their own list; the index is the only thing a button carries.
	_, rkb := telegram.RenderEmojiStep(bundle, "es", telegram.EmojiSuggestions(bundle, "es", "picker.emoji_suggestions_reminders"))
	if got := rkb.InlineKeyboard[0][3]; got.Text != "💡" || got.CallbackData != "fl:em:3" {
		t.Errorf("reminder suggestion 3: got %+v", got)
	}
}

// chooseChoreName runs /add in the Chores topic and types the name, leaving the emoji step active.
func chooseChoreName(ctx context.Context, t *testing.T, env *testEnv, name string) {
	t.Helper()
	if err := polishChoreText(ctx, env, 1, 101, "/add"); err != nil {
		t.Fatalf("/add failed: %v", err)
	}
	if err := polishChoreText(ctx, env, 2, 101, name); err != nil {
		t.Fatalf("name failed: %v", err)
	}
	if last := env.client.LastSent(); last == nil || !strings.Contains(last.Text, env.i18n.T("en", "picker.prompt_emoji")) {
		t.Fatalf("expected emoji step after the name, got %+v", last)
	}
}

// flowState returns the flow step and payload for the Chores topic of user 101.
func flowState(ctx context.Context, t *testing.T, env *testEnv, threadID int64) (string, map[string]any) {
	t.Helper()
	var payload map[string]any
	state, err := env.flow.Get(ctx, flowKeyFor(101, staleTestChatID, threadID), &payload)
	if err != nil || state == nil {
		t.Fatalf("expected an active flow, got state=%v err=%v", state, err)
	}
	return state.Step, payload
}

func TestChoreEmojiStep_StoresChosenEmoji(t *testing.T) {
	cases := []struct {
		name      string
		press     string
		typed     []string
		wantEmoji string
	}{
		{"suggestion", "fl:em:2", nil, "🚰"},
		{"skip keeps default", "fl:em:skip", nil, "🧹"},
		{"typed emoji", "fl:em:o", []string{"🪴"}, "🪴"},
		{"invalid then valid typed", "fl:em:o", []string{"plants", "🪴"}, "🪴"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env, err := setupTestEnv()
			if err != nil {
				t.Fatalf("setup failed: %v", err)
			}
			ctx := context.Background()
			env.clock.now = time.Date(2026, 10, 5, 14, 20, 0, 0, env.clock.loc)

			chooseChoreName(ctx, t, env, "Sweep patio")
			if err := pressChoreCallback(ctx, env, 10, 101, c.press); err != nil {
				t.Fatalf("%s failed: %v", c.press, err)
			}
			for i, text := range c.typed {
				if err := polishChoreText(ctx, env, int64(20+i), 101, text); err != nil {
					t.Fatalf("typed %q failed: %v", text, err)
				}
			}

			steps := []string{"fl:usr:101", "fl:rt:once", "fl:d:2026-10-14", "fl:h:10", "fl:mi:00"}
			for i, data := range steps {
				if err := pressChoreCallback(ctx, env, int64(40+i), 101, data); err != nil {
					t.Fatalf("%s failed: %v", data, err)
				}
			}
			edit := env.client.LastEdited()
			if edit == nil || !strings.Contains(edit.Text, c.wantEmoji+" Sweep patio") {
				t.Fatalf("expected confirmation with %s in the header and summary, got %+v", c.wantEmoji, edit)
			}
			if err := pressChoreCallback(ctx, env, 50, 101, "fl:ok"); err != nil {
				t.Fatalf("confirm failed: %v", err)
			}

			defs, err := env.chores.ListDefinitions(ctx)
			if err != nil {
				t.Fatalf("list definitions: %v", err)
			}
			if len(defs) != 1 || defs[0].Name != "Sweep patio" || defs[0].Emoji != c.wantEmoji {
				t.Fatalf("expected stored definition with %s, got %+v", c.wantEmoji, defs)
			}
		})
	}
}

func TestChoreEmojiStep_InvalidTypedAnswerAsksAgain(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	ctx := context.Background()
	chooseChoreName(ctx, t, env, "Sweep patio")
	if err := pressChoreCallback(ctx, env, 10, 101, "fl:em:o"); err != nil {
		t.Fatalf("Other failed: %v", err)
	}

	if err := polishChoreText(ctx, env, 20, 101, "plants"); err != nil {
		t.Fatalf("typed answer failed: %v", err)
	}
	sent := env.client.LastSent()
	if sent == nil || !strings.Contains(sent.Text, env.i18n.T("en", "picker.invalid_emoji")) {
		t.Fatalf("expected the rejection notice, got %+v", sent)
	}
	if step, _ := flowState(ctx, t, env, 51); step != "chore_typed_emoji" {
		t.Fatalf("expected the typed emoji step to stay active, got %s", step)
	}
	if defs, _ := env.chores.ListDefinitions(ctx); len(defs) != 0 {
		t.Fatalf("an invalid emoji must store nothing, got %+v", defs)
	}
}

func TestChoreEmojiStep_OutOfRangeIndexChangesNothing(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	ctx := context.Background()
	chooseChoreName(ctx, t, env, "Sweep patio")

	// Eight suggestions exist, so index 8 is outside the locale list.
	if err := pressChoreCallback(ctx, env, 10, 101, "fl:em:8"); err != nil {
		t.Fatalf("out-of-range press failed: %v", err)
	}
	if step, payload := flowState(ctx, t, env, 51); step != "chore_emoji" || payload["emoji"] != nil {
		t.Fatalf("out-of-range index must leave the emoji step unchanged, got step=%s payload=%v", step, payload)
	}
}

func TestChoreEmojiStep_ExistingChoreKeepsEmoji(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	ctx := context.Background()
	def, err := env.chores.CreateDefinition(ctx, 101, "Lavar el baño", "🚿", "", nil)
	if err != nil {
		t.Fatalf("create definition: %v", err)
	}

	if err := polishChoreText(ctx, env, 1, 101, "/add"); err != nil {
		t.Fatalf("/add failed: %v", err)
	}
	if err := pressChoreCallback(ctx, env, 2, 101, "fl:sel:"+strconv.FormatInt(def.ID, 10)); err != nil {
		t.Fatalf("select failed: %v", err)
	}
	if step, payload := flowState(ctx, t, env, 51); step != "chore_assignee" || payload["emoji"] != "🚿" {
		t.Fatalf("expected assignee step with 🚿 kept, got step=%s payload=%v", step, payload)
	}
	for _, sent := range env.client.SentMessages() {
		if strings.Contains(sent.Text, env.i18n.T("en", "picker.prompt_emoji")) {
			t.Fatalf("an existing chore must not ask for an emoji, got %q", sent.Text)
		}
	}
}

func TestReminderEmojiStep_StoresChosenEmoji(t *testing.T) {
	cases := []struct {
		name      string
		press     string
		typed     string
		wantEmoji string
	}{
		{"suggestion", "fl:em:3", "", "💡"},
		{"skip keeps default", "fl:em:skip", "", "⏰"},
		{"typed emoji", "fl:em:o", "🪴", "🪴"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env, err := setupTestEnv()
			if err != nil {
				t.Fatalf("setup failed: %v", err)
			}
			ctx := context.Background()
			env.clock.now = time.Date(2026, 10, 5, 14, 20, 0, 0, env.clock.loc)

			if err := sendText(ctx, env, 1, 101, 63, "/add"); err != nil {
				t.Fatalf("/add failed: %v", err)
			}
			if err := sendText(ctx, env, 2, 101, 63, "Pay power bill"); err != nil {
				t.Fatalf("title failed: %v", err)
			}
			if last := env.client.LastSent(); last == nil || !strings.Contains(last.Text, env.i18n.T("en", "picker.prompt_emoji")) {
				t.Fatalf("expected emoji step after the title, got %+v", last)
			}
			if err := pressReminderCallback(ctx, env, 10, 101, c.press); err != nil {
				t.Fatalf("%s failed: %v", c.press, err)
			}
			if c.typed != "" {
				if err := sendText(ctx, env, 20, 101, 63, c.typed); err != nil {
					t.Fatalf("typed emoji failed: %v", err)
				}
			}

			steps := []string{"fl:usr:102", "fl:rt:once", "fl:d:2026-10-14", "fl:h:10", "fl:mi:00"}
			for i, data := range steps {
				if err := pressReminderCallback(ctx, env, int64(40+i), 101, data); err != nil {
					t.Fatalf("%s failed: %v", data, err)
				}
			}
			if err := pressReminderCallback(ctx, env, 50, 101, "fl:ok"); err != nil {
				t.Fatalf("confirm failed: %v", err)
			}

			defs, err := env.reminders.ListDefinitions(ctx)
			if err != nil {
				t.Fatalf("list definitions: %v", err)
			}
			if len(defs) != 1 || defs[0].Title != "Pay power bill" || defs[0].Emoji != c.wantEmoji {
				t.Fatalf("expected stored reminder with %s, got %+v", c.wantEmoji, defs)
			}
		})
	}
}
