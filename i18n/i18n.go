package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"strings"
	"sync"
)

//go:embed locales/*.json
var localesFS embed.FS

const defaultLang = "en"

var (
	translations map[string]map[string]string
	once         sync.Once
)

func load() {
	once.Do(func() {
		translations = make(map[string]map[string]string)

		entries, err := localesFS.ReadDir("locales")
		if err != nil {
			slog.Error("i18n: failed to read locales directory", "error", err)
			return
		}

		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}

			data, err := localesFS.ReadFile("locales/" + entry.Name())
			if err != nil {
				slog.Error("i18n: failed to read locale file", "file", entry.Name(), "error", err)
				continue
			}

			var messages map[string]string
			if err := json.Unmarshal(data, &messages); err != nil {
				slog.Error("i18n: failed to parse locale file", "file", entry.Name(), "error", err)
				continue
			}

			lang := strings.TrimSuffix(entry.Name(), ".json")
			translations[lang] = messages
		}
	})
}

// T returns the translated string for the given language and key.
// Falls back to English if key is missing in the target language.
// Logs a warning when falling back.
// Supports fmt.Sprintf-style interpolation when args are provided.
func T(lang, key string, args ...any) string {
	load()

	if msgs, ok := translations[lang]; ok {
		if val, ok := msgs[key]; ok {
			if len(args) > 0 {
				return fmt.Sprintf(val, args...)
			}
			return val
		}
	}

	// Fallback to default language
	if lang != defaultLang {
		slog.Warn("i18n: missing translation, falling back to default", "lang", lang, "key", key)
		if msgs, ok := translations[defaultLang]; ok {
			if val, ok := msgs[key]; ok {
				if len(args) > 0 {
					return fmt.Sprintf(val, args...)
				}
				return val
			}
		}
	}

	slog.Warn("i18n: missing translation key", "lang", lang, "key", key)
	return key
}

// FuncMap returns a template.FuncMap with the "t" function registered.
// Usage in templates: {{t .Lang "nav.topics"}}
func FuncMap() template.FuncMap {
	return template.FuncMap{
		"t": func(lang, key string, args ...any) string {
			return T(lang, key, args...)
		},
	}
}

// SupportedLanguages returns the list of available language codes.
func SupportedLanguages() []string {
	load()
	langs := make([]string, 0, len(translations))
	for lang := range translations {
		langs = append(langs, lang)
	}
	return langs
}

// IsSupported returns true if the given language code is supported.
func IsSupported(lang string) bool {
	load()
	_, ok := translations[lang]
	return ok
}

// MatchLanguage parses an Accept-Language header and returns the best matching
// supported language. Returns the default language if no match is found.
func MatchLanguage(acceptHeader string) string {
	load()

	if acceptHeader == "" {
		return defaultLang
	}

	// Parse Accept-Language: e.g. "pt-BR,pt;q=0.9,en-US;q=0.8,en;q=0.7"
	for _, part := range strings.Split(acceptHeader, ",") {
		part = strings.TrimSpace(part)
		// Strip quality value
		if idx := strings.Index(part, ";"); idx >= 0 {
			part = part[:idx]
		}
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Exact match
		if _, ok := translations[part]; ok {
			return part
		}

		// Prefix match: "pt" matches "pt-BR"
		for lang := range translations {
			if strings.HasPrefix(lang, part+"-") || strings.HasPrefix(part, lang+"-") {
				return lang
			}
		}
	}

	return defaultLang
}

// DefaultLanguage returns the default language code.
func DefaultLanguage() string {
	return defaultLang
}
