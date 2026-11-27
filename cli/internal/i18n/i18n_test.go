package i18n

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"testing/fstest"

	"golang.org/x/text/language"
)

// sourceMessages is the frontend translation tree, relative to this package.
const sourceMessages = "../../../apps/frontend/messages"

// The embedded copy must equal the server sections of the frontend
// translations. Run `mise sync-server-messages` after editing them.
func TestEmbeddedMessagesMatchFrontendSource(t *testing.T) {
	want := map[string][]byte{}
	sources, err := filepath.Glob(filepath.Join(sourceMessages, "*", "server", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range sources {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		locale := filepath.Base(filepath.Dir(filepath.Dir(file)))
		want[locale+"/"+filepath.Base(file)] = data
	}
	got := map[string][]byte{}
	err = fs.WalkDir(embedded, "messages", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := fs.ReadFile(embedded, path)
		got[path[len("messages/"):]] = data
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range want {
		if !bytes.Equal(got[name], data) {
			t.Errorf("%s differs from apps/frontend/messages; run `mise sync-server-messages`", name)
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			t.Errorf("%s has no source in apps/frontend/messages; run `mise sync-server-messages`", name)
		}
	}
}

// Every frontend locale without a regional fallback needs the server
// catalogs. Without them, the server would write English for that locale.
func TestEveryFrontendLocaleHasServerCatalogs(t *testing.T) {
	entries, err := os.ReadDir(sourceMessages)
	if err != nil {
		t.Fatal(err)
	}
	base, err := filepath.Glob(filepath.Join(sourceMessages, BaseLocale, "server", "*.json"))
	if err != nil || len(base) == 0 {
		t.Fatalf("no %s server catalogs: %v", BaseLocale, err)
	}
	for _, entry := range entries {
		locale := entry.Name()
		if !entry.IsDir() || fallbackLocales[locale] != "" {
			continue
		}
		for _, file := range base {
			if _, err := os.Stat(filepath.Join(sourceMessages, locale, "server", filepath.Base(file))); err != nil {
				t.Errorf("%s has no server/%s", locale, filepath.Base(file))
			}
		}
	}
}

// Every locale must translate exactly the keys of the base catalog, with the
// same placeholders, so that a missing or misspelled key cannot fall back to
// English unnoticed.
func TestCatalogsMatchBaseKeysAndPlaceholders(t *testing.T) {
	c := Default()
	base := c.messages[BaseLocale]
	for _, locale := range c.locales {
		if _, sparse := fallbackLocales[locale]; sparse && len(c.messages[locale]) == 0 {
			continue
		}
		for key, msg := range c.messages[locale] {
			want, ok := base[key]
			if !ok {
				t.Errorf("%s: key %q is not in %s", locale, key, BaseLocale)
				continue
			}
			if got, wantSet := placeholders(msg), placeholders(want); !equal(got, wantSet) {
				t.Errorf("%s: %q has placeholders %v, want %v", locale, key, got, wantSet)
			}
		}
		if _, sparse := fallbackLocales[locale]; sparse {
			continue
		}
		for key := range base {
			if _, ok := c.messages[locale][key]; !ok {
				t.Errorf("%s: missing key %q", locale, key)
			}
		}
	}
}

// A plural message must have every CLDR category that its language uses for
// whole numbers. A missing category would fall back to "other" and give the
// wrong grammar, for example "5 минуты" instead of "5 минут".
func TestPluralMessagesHaveTheLanguagesCategories(t *testing.T) {
	c := Default()
	for _, locale := range c.locales {
		tag := language.MustParse(locale)
		needed := map[string]bool{}
		for n := 0; n <= 1000; n++ {
			needed[pluralCategory(tag, n)] = true
		}
		for key, msg := range c.messages[locale] {
			if msg.plural == nil {
				continue
			}
			for category := range needed {
				if msg.plural[category] == "" {
					t.Errorf("%s: %q lacks the %q form", locale, key, category)
				}
			}
		}
	}
}

func placeholders(msg message) []string {
	texts := []string{msg.text}
	for _, form := range msg.plural {
		texts = append(texts, form)
	}
	seen := map[string]bool{}
	for _, text := range texts {
		for i := 0; i < len(text); i++ {
			if text[i] != '{' {
				continue
			}
			end := bytes.IndexByte([]byte(text[i:]), '}')
			if end > 0 {
				seen[text[i+1:i+end]] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func testCatalog(t *testing.T) *Catalog {
	t.Helper()
	files := fstest.MapFS{
		"m/en-GB/s.json": {Data: []byte(`{"s":{"hello":"Hello {name}","only_en":"English","n":{"one":"{count} item","other":"{count} items"}}}`)},
		"m/de-DE/s.json": {Data: []byte(`{"s":{"hello":"Hallo {name}","n":{"one":"{count} Eintrag","other":"{count} Einträge"}}}`)},
		"m/ru-RU/s.json": {Data: []byte(`{"s":{"n":{"one":"{count} элемент","few":"{count} элемента","many":"{count} элементов","other":"{count} элемента"}}}`)},
		"m/ar/s.json":    {Data: []byte(`{"s":{"hello":"مرحبا {name}"}}`)},
	}
	c, err := Load(files, "m")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestTranslatorFallsBackPerKey(t *testing.T) {
	c := testCatalog(t)
	tests := []struct {
		locale, key, want string
	}{
		{"de-DE", "s.hello", "Hallo Ada"},
		{"de-DE", "s.only_en", "English"},
		// de-AT has no files; it falls back to de-DE, then en-GB.
		{"de-AT", "s.hello", "Hallo Ada"},
		{"xx-YY", "s.hello", "Hello Ada"},
		{"de-DE", "s.missing", "s.missing"},
	}
	for _, tt := range tests {
		if got := c.Translator(tt.locale).T(tt.key, Args{"name": "Ada"}); got != tt.want {
			t.Errorf("%s %s = %q, want %q", tt.locale, tt.key, got, tt.want)
		}
	}
}

func TestTranslatorPlurals(t *testing.T) {
	c := testCatalog(t)
	tests := []struct {
		locale string
		count  int
		want   string
	}{
		{"en-GB", 1, "1 item"},
		{"en-GB", 30, "30 items"},
		{"de-DE", 1, "1 Eintrag"},
		{"ru-RU", 1, "1 элемент"},
		{"ru-RU", 2, "2 элемента"},
		{"ru-RU", 5, "5 элементов"},
		{"ru-RU", 21, "21 элемент"},
	}
	for _, tt := range tests {
		if got := c.Translator(tt.locale).N("s.n", tt.count, nil); got != tt.want {
			t.Errorf("%s N(%d) = %q, want %q", tt.locale, tt.count, got, tt.want)
		}
	}
}

func TestMatchAcceptLanguage(t *testing.T) {
	c := testCatalog(t)
	tests := map[string]string{
		"":                          BaseLocale,
		"not a header;;;":           BaseLocale,
		"de-DE":                     "de-DE",
		"de":                        "de-DE",
		"de-AT,de;q=0.9":            "de-AT",
		"fr-FR,de;q=0.5":            "de-DE",
		"fr-FR":                     BaseLocale,
		"ar-EG,en;q=0.5":            "ar",
		"ru;q=0.2,de;q=0.4,en;q=.1": "de-DE",
	}
	for header, want := range tests {
		if got := c.Match(header); got != want {
			t.Errorf("Match(%q) = %q, want %q", header, got, want)
		}
	}
}

func TestTranslatorDirection(t *testing.T) {
	c := testCatalog(t)
	for locale, want := range map[string]string{"ar": "rtl", "de-DE": "ltr", "en-GB": "ltr"} {
		if got := c.Translator(locale).Dir(); got != want {
			t.Errorf("Dir(%s) = %q, want %q", locale, got, want)
		}
	}
}

func TestLoadRejectsInvalidCatalogs(t *testing.T) {
	for name, files := range map[string]fstest.MapFS{
		"no base":      {"m/de-DE/s.json": {Data: []byte(`{}`)}},
		"invalid json": {"m/en-GB/s.json": {Data: []byte(`{`)}},
		"bad value":    {"m/en-GB/s.json": {Data: []byte(`{"s":{"n":1}}`)}},
	} {
		if _, err := Load(files, "m"); err == nil {
			t.Errorf("%s: Load succeeded", name)
		}
	}
}

func TestDefaultCatalogMatchesBrowserLanguages(t *testing.T) {
	tests := map[string]string{
		"de-DE,de;q=0.9,en;q=0.8": "de-DE",
		"de-AT":                   "de-AT",
		"pt":                      "pt-BR",
		"pt-PT":                   "pt-PT",
		"zh-Hant":                 "zh-TW",
		"no":                      "nb-NO",
		"es-MX":                   "es-419",
		"iw":                      "he-IL",
		"fi":                      BaseLocale,
	}
	for header, want := range tests {
		if got := Default().Match(header); got != want {
			t.Errorf("Match(%q) = %q, want %q", header, got, want)
		}
	}
}
