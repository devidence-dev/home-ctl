package i18n_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/px1/home-ctl/internal/i18n"
)

func writeTempLocale(t *testing.T, dir, filename, content string) {
	t.Helper()
	err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0600)
	if err != nil {
		t.Fatalf("failed to write %s: %v", filename, err)
	}
}

func TestI18n_Scenarios(t *testing.T) {
	tempDir := t.TempDir()

	enContent := `
buttons:
  done: "Done"
  postpone: "Postpone"
notifications:
  chore_due: "Chore due: %s"
fallback_only: "Only in English"
`
	esContent := `
buttons:
  done: "Hecho"
  postpone: "Posponer"
notifications:
  chore_due: "Tarea pendiente: %s"
`
	writeTempLocale(t, tempDir, "en.yaml", enContent)
	writeTempLocale(t, tempDir, "es.yaml", esContent)

	requiredKeys := []string{"buttons.done", "buttons.postpone", "notifications.chore_due"}

	// 1. Valid bundle initialization
	bundle, err := i18n.NewBundle(tempDir, "en", requiredKeys, nil)
	if err != nil {
		t.Fatalf("failed to create bundle: %v", err)
	}

	// 2. Spanish member rendering
	esDone := bundle.T("es", "buttons.done")
	if esDone != "Hecho" {
		t.Fatalf("expected 'Hecho', got %q", esDone)
	}
	esChore := bundle.T("es", "notifications.chore_due", "Sweep")
	if esChore != "Tarea pendiente: Sweep" {
		t.Fatalf("expected Spanish chore formatting, got %q", esChore)
	}

	// 3. Fallback to English when key missing in es.yaml
	fallback := bundle.T("es", "fallback_only")
	if fallback != "Only in English" {
		t.Fatalf("expected fallback to English 'Only in English', got %q", fallback)
	}

	// 4. Missing required key at startup fails naming the key
	badRequired := []string{"buttons.done", "nonexistent.required_key"}
	_, err = i18n.NewBundle(tempDir, "en", badRequired, nil)
	if err == nil {
		t.Fatalf("expected error for missing required key, got nil")
	}
	if !strings.Contains(err.Error(), "nonexistent.required_key") {
		t.Fatalf("expected error naming missing key, got %v", err)
	}

	// 5. Unsupported default locale fails naming the locale
	_, err = i18n.NewBundle(tempDir, "fr", requiredKeys, nil)
	if err == nil {
		t.Fatalf("expected error for unsupported default locale fr, got nil")
	}
	if !strings.Contains(err.Error(), "fr") {
		t.Fatalf("expected error naming 'fr', got %v", err)
	}

	// 6. Missing en.yaml
	emptyDir := t.TempDir()
	writeTempLocale(t, emptyDir, "es.yaml", esContent)
	_, err = i18n.NewBundle(emptyDir, "en", requiredKeys, nil)
	if err == nil {
		t.Fatalf("expected error when en.yaml is missing")
	}
}

// Every English key must exist in Spanish with the same format verbs, and vice versa,
// so a locale never silently falls back to English or crashes on a verb mismatch.
func TestShippedLocalesAreInSync(t *testing.T) {
	load := func(name string) map[string]string {
		raw, err := fs.ReadFile(os.DirFS(filepath.Join("..", "..", "locales")), name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		var parsed map[string]interface{}
		if err := yaml.Unmarshal(raw, &parsed); err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		flat := map[string]string{}
		var flatten func(prefix string, m map[string]interface{})
		flatten = func(prefix string, m map[string]interface{}) {
			for k, v := range m {
				p := k
				if prefix != "" {
					p = prefix + "." + k
				}
				switch val := v.(type) {
				case map[string]interface{}:
					flatten(p, val)
				case string:
					flat[p] = val
				}
			}
		}
		flatten("", parsed)
		return flat
	}
	verbs := func(s string) string {
		found := regexp.MustCompile(`%[0-9]*(\[\d+\])?[a-z]`).FindAllString(s, -1)
		sort.Strings(found) // word order may differ between languages
		return strings.Join(found, ",")
	}
	en, es := load("en.yaml"), load("es.yaml")
	for key, text := range en {
		other, ok := es[key]
		if !ok {
			t.Errorf("key %q missing in es.yaml", key)
			continue
		}
		if verbs(text) != verbs(other) {
			t.Errorf("key %q: format verbs differ (en %q, es %q)", key, verbs(text), verbs(other))
		}
	}
	for key := range es {
		if _, ok := en[key]; !ok {
			t.Errorf("key %q exists only in es.yaml", key)
		}
	}
}

func TestFormatDateTime_LocalizedAndTimezoneAware(t *testing.T) {
	b, err := i18n.NewBundle(filepath.Join("..", "..", "locales"), "en", i18n.DefaultRequiredKeys, nil)
	if err != nil {
		t.Fatalf("NewBundle failed: %v", err)
	}
	loc, err := time.LoadLocation("America/Guayaquil") // UTC-5, no DST
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	// 23:30 local on Mon Oct 5 2026 is already Tue Oct 6 04:30 UTC.
	now := time.Date(2026, time.October, 5, 23, 30, 0, 0, loc)

	cases := []struct {
		name, locale string
		at           time.Time
		want         string
	}{
		{"today", "en", time.Date(2026, 10, 5, 18, 0, 0, 0, loc), "today at 18:00"},
		{"tomorrow en", "en", time.Date(2026, 10, 6, 9, 0, 0, 0, loc), "tomorrow at 09:00"},
		{"tomorrow es", "es", time.Date(2026, 10, 6, 9, 0, 0, 0, loc), "mañana a las 09:00"},
		{"yesterday en", "en", time.Date(2026, 10, 4, 18, 0, 0, 0, loc), "yesterday at 18:00"},
		{"yesterday es", "es", time.Date(2026, 10, 4, 18, 0, 0, 0, loc), "ayer a las 18:00"},
		{"later en", "en", time.Date(2026, 10, 10, 10, 5, 0, 0, loc), "Sat, Oct 10, 10:05"},
		{"later es", "es", time.Date(2026, 10, 10, 10, 5, 0, 0, loc), "sáb 10 oct, 10:05"},
		{"other year es", "es", time.Date(2027, 1, 4, 8, 0, 0, 0, loc), "lun 4 ene 2027, 08:00"},
		// Stored in UTC: 04:30 UTC Oct 6 is 23:30 Oct 5 in Guayaquil, i.e. "today".
		{"utc input uses household tz", "en", time.Date(2026, 10, 6, 4, 30, 0, 0, time.UTC), "today at 23:30"},
		{"unknown locale falls back to default", "fr", time.Date(2026, 10, 10, 10, 5, 0, 0, loc), "Sat, Oct 10, 10:05"},
	}
	for _, c := range cases {
		if got := b.FormatDateTime(c.locale, c.at, now, loc); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestFormatDateTime_MalformedNamesFallBackToEnglishAbbreviations(t *testing.T) {
	dir := t.TempDir()
	writeTempLocale(t, dir, "en.yaml", `
datetime:
  weekdays: "only,three,names"
  months: "bad"
  full: "%[1]s %[2]d %[3]s %02[4]d:%02[5]d"
  full_year: "%[1]s %[2]d %[3]s %[6]d"
  today: "today %s"
  tomorrow: "tomorrow %s"
`)
	b, err := i18n.NewBundle(dir, "en", nil, nil)
	if err != nil {
		t.Fatalf("NewBundle failed: %v", err)
	}
	now := time.Date(2026, time.October, 1, 8, 0, 0, 0, time.UTC)
	at := time.Date(2026, time.October, 10, 10, 5, 0, 0, time.UTC)
	if got, want := b.FormatDateTime("en", at, now, time.UTC), "Sat 10 Oct 10:05"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestBundle_Locales(t *testing.T) {
	b, err := i18n.NewBundle(filepath.Join("..", "..", "locales"), "en", i18n.DefaultRequiredKeys, nil)
	if err != nil {
		t.Fatalf("NewBundle failed: %v", err)
	}
	locales := b.Locales()
	if len(locales) != 2 || locales[0] != "en" || locales[1] != "es" {
		t.Fatalf("expected [en es], got %v", locales)
	}
}

func TestBundle_MissingCommandKeyFailsStartup(t *testing.T) {
	tempDir := t.TempDir()
	enContent := `
buttons:
  done: "Done"
`
	writeTempLocale(t, tempDir, "en.yaml", enContent)
	_, err := i18n.NewBundle(tempDir, "en", i18n.DefaultRequiredKeys, nil)
	if err == nil {
		t.Fatalf("expected error for missing required keys, got nil")
	}

	badKeys := []string{"telegram.commands.add.description"}
	_, err = i18n.NewBundle(tempDir, "en", badKeys, nil)
	if err == nil {
		t.Fatalf("expected error for missing telegram.commands.add.description, got nil")
	}
	if !strings.Contains(err.Error(), "telegram.commands.add.description") {
		t.Fatalf("expected error naming missing key, got %v", err)
	}
}

func TestBundle_MissingPickerKeyFailsStartup(t *testing.T) {
	tempDir := t.TempDir()
	enContent := `
buttons:
  done: "Done"
`
	writeTempLocale(t, tempDir, "en.yaml", enContent)
	badKeys := []string{"picker.prompt_recurrence"}
	_, err := i18n.NewBundle(tempDir, "en", badKeys, nil)
	if err == nil {
		t.Fatalf("expected error for missing picker.prompt_recurrence, got nil")
	}
	if !strings.Contains(err.Error(), "picker.prompt_recurrence") {
		t.Fatalf("expected error naming missing key, got %v", err)
	}
}
