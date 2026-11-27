// Package i18n translates texts that the Chatto server writes, such as
// transactional email.
//
// The catalogs are the server sections of the frontend translations
// (apps/frontend/messages/<locale>/server/*.json). They use the Translatto
// format: each file holds one section as nested JSON objects. A leaf is a
// string with {placeholders}, or a plural object with CLDR categories
// (zero, one, two, few, many, other) that is selected by the count argument.
// `mise sync-server-messages` copies the files into messages/ so that the
// binary can embed them. A test fails when the copy and the source differ.
//
// Lookups use the same fallback as the frontend: the requested locale, then
// its regional fallback (for example de-AT to de-DE), then en-GB, separately
// for each key.
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"path"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/text/feature/plural"
	"golang.org/x/text/language"
)

// BaseLocale is the locale of the source catalogs. Every key must exist in
// it.
const BaseLocale = "en-GB"

// fallbackLocales mirrors the sparse regional locales of the frontend
// (apps/frontend/src/lib/i18n/locales.ts).
var fallbackLocales = map[string]string{
	"en-US": "en-GB",
	"de-AT": "de-DE",
	"de-CH": "de-DE",
}

// rtlLanguages are the base languages of the supported locales that are
// written right to left.
var rtlLanguages = map[string]bool{"ar": true, "he": true, "fa": true, "ur": true}

//go:embed messages
var embedded embed.FS

var defaultCatalog = mustLoad(embedded, "messages")

// Default returns the catalog of the embedded server messages.
func Default() *Catalog { return defaultCatalog }

// Catalog holds the translations of all locales.
type Catalog struct {
	// messages maps a locale to its flattened keys ("email.subject").
	messages map[string]map[string]message
	// locales lists the locales that can be selected, BaseLocale first.
	locales []string
	matcher language.Matcher
}

// message is one leaf: a string, or the forms of a plural message.
type message struct {
	text   string
	plural map[string]string
}

// Load reads catalogs from dir/<locale>/<section>.json in fsys. It returns
// an error when a file is not valid or BaseLocale is missing.
func Load(fsys fs.FS, dir string) (*Catalog, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, err
	}
	c := &Catalog{messages: map[string]map[string]message{}}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		locale := entry.Name()
		if _, err := language.Parse(locale); err != nil {
			return nil, fmt.Errorf("invalid locale directory %q: %w", locale, err)
		}
		files, err := fs.Glob(fsys, path.Join(dir, locale, "*.json"))
		if err != nil {
			return nil, err
		}
		messages := map[string]message{}
		for _, file := range files {
			data, err := fs.ReadFile(fsys, file)
			if err != nil {
				return nil, err
			}
			var doc map[string]any
			if err := json.Unmarshal(data, &doc); err != nil {
				return nil, fmt.Errorf("%s: %w", file, err)
			}
			if err := flatten(doc, "", messages); err != nil {
				return nil, fmt.Errorf("%s: %w", file, err)
			}
		}
		c.messages[locale] = messages
	}
	if _, ok := c.messages[BaseLocale]; !ok {
		return nil, fmt.Errorf("missing %s catalog", BaseLocale)
	}
	// A locale without own files can still be selected when its fallback has
	// them, as the frontend does for sparse regional locales.
	for locale, fallback := range fallbackLocales {
		if _, ok := c.messages[locale]; !ok && c.messages[fallback] != nil {
			c.messages[locale] = map[string]message{}
		}
	}
	for locale := range c.messages {
		if locale != BaseLocale {
			c.locales = append(c.locales, locale)
		}
	}
	sort.Strings(c.locales)
	c.locales = append([]string{BaseLocale}, c.locales...)
	tags := make([]language.Tag, len(c.locales))
	for i, locale := range c.locales {
		tags[i] = language.MustParse(locale)
	}
	c.matcher = language.NewMatcher(tags)
	return c, nil
}

func mustLoad(fsys fs.FS, dir string) *Catalog {
	c, err := Load(fsys, dir)
	if err != nil {
		panic("invalid embedded server messages: " + err.Error())
	}
	return c
}

// flatten adds the leaves of doc to messages under dotted keys.
func flatten(doc map[string]any, prefix string, messages map[string]message) error {
	for key, value := range doc {
		full := key
		if prefix != "" {
			full = prefix + "." + key
		}
		switch v := value.(type) {
		case string:
			messages[full] = message{text: v}
		case map[string]any:
			if forms, ok := pluralForms(v); ok {
				messages[full] = message{plural: forms}
				continue
			}
			if err := flatten(v, full, messages); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%s: unsupported value %T", full, value)
		}
	}
	return nil
}

// pluralForms reports whether v is a plural message: only CLDR category keys
// with string values, including "other".
func pluralForms(v map[string]any) (map[string]string, bool) {
	forms := map[string]string{}
	for key, value := range v {
		text, ok := value.(string)
		if !ok || !isPluralCategory(key) {
			return nil, false
		}
		forms[key] = text
	}
	_, hasOther := forms["other"]
	return forms, hasOther
}

func isPluralCategory(key string) bool {
	switch key {
	case "zero", "one", "two", "few", "many", "other":
		return true
	}
	return false
}

// Locales returns the locales that Match can select, BaseLocale first.
func (c *Catalog) Locales() []string { return append([]string(nil), c.locales...) }

// Match returns the supported locale that best fits an Accept-Language header
// value. It returns BaseLocale for an empty or invalid value or when no
// language fits.
func (c *Catalog) Match(acceptLanguage string) string {
	tags, _, err := language.ParseAcceptLanguage(acceptLanguage)
	if err != nil || len(tags) == 0 {
		return BaseLocale
	}
	_, index, confidence := c.matcher.Match(tags...)
	if confidence == language.No {
		return BaseLocale
	}
	return c.locales[index]
}

// Translator returns translations for locale. An unsupported locale uses
// BaseLocale.
func (c *Catalog) Translator(locale string) Translator {
	if _, ok := c.messages[locale]; !ok {
		locale = BaseLocale
	}
	chain := []string{locale}
	if fallback, ok := fallbackLocales[locale]; ok {
		chain = append(chain, fallback)
	}
	if locale != BaseLocale {
		chain = append(chain, BaseLocale)
	}
	return Translator{catalog: c, chain: chain, tag: language.MustParse(locale)}
}

// Translator looks up messages for one locale.
type Translator struct {
	catalog *Catalog
	chain   []string
	tag     language.Tag
}

// Args are the values of a message's {placeholders}.
type Args map[string]string

// Locale returns the selected locale, for example "de-DE".
func (t Translator) Locale() string { return t.chain[0] }

// Dir returns "rtl" for a language that is written right to left, and "ltr"
// for others.
func (t Translator) Dir() string {
	base, _ := t.tag.Base()
	if rtlLanguages[base.String()] {
		return "rtl"
	}
	return "ltr"
}

// T returns the message for key with its placeholders replaced. A missing key
// returns the key, so that a missing translation is visible but not fatal.
func (t Translator) T(key string, args Args) string {
	msg, ok := t.lookup(key)
	if !ok {
		return key
	}
	text := msg.text
	if msg.plural != nil {
		text = msg.plural["other"]
	}
	return interpolate(text, args)
}

// N returns the plural message for key in the form that count needs in this
// locale. {count} is replaced with count.
func (t Translator) N(key string, count int, args Args) string {
	msg, ok := t.lookup(key)
	if !ok {
		return key
	}
	text := msg.text
	if msg.plural != nil {
		text = msg.plural[pluralCategory(t.tag, count)]
		if text == "" {
			text = msg.plural["other"]
		}
	}
	all := Args{"count": strconv.Itoa(count)}
	maps.Copy(all, args)
	return interpolate(text, all)
}

func (t Translator) lookup(key string) (message, bool) {
	for _, locale := range t.chain {
		if msg, ok := t.catalog.messages[locale][key]; ok {
			return msg, true
		}
	}
	return message{}, false
}

func pluralCategory(tag language.Tag, count int) string {
	n := count
	if n < 0 {
		n = -n
	}
	switch plural.Cardinal.MatchPlural(tag, n, 0, 0, 0, 0) {
	case plural.Zero:
		return "zero"
	case plural.One:
		return "one"
	case plural.Two:
		return "two"
	case plural.Few:
		return "few"
	case plural.Many:
		return "many"
	default:
		return "other"
	}
}

// interpolate replaces {name} with args[name]. Unknown placeholders stay.
func interpolate(text string, args Args) string {
	if len(args) == 0 || !strings.Contains(text, "{") {
		return text
	}
	pairs := make([]string, 0, len(args)*2)
	for name, value := range args {
		pairs = append(pairs, "{"+name+"}", value)
	}
	return strings.NewReplacer(pairs...).Replace(text)
}
