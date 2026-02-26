package i18n

import (
	"testing"
)

func TestT_ReturnsEnglishTranslation(t *testing.T) {
	got := T("en", "login.title")
	want := "Welcome to bobot"
	if got != want {
		t.Errorf("T(en, login.title) = %q, want %q", got, want)
	}
}

func TestT_ReturnsPtBRTranslation(t *testing.T) {
	got := T("pt-BR", "login.title")
	want := "Bem-vindo ao bobot"
	if got != want {
		t.Errorf("T(pt-BR, login.title) = %q, want %q", got, want)
	}
}

func TestT_FallbackToEnglish(t *testing.T) {
	// Request a key with an unsupported language - should fall back to English
	got := T("fr", "login.title")
	want := "Welcome to bobot"
	if got != want {
		t.Errorf("T(fr, login.title) = %q, want %q (expected English fallback)", got, want)
	}
}

func TestT_MissingKeyReturnsKey(t *testing.T) {
	got := T("en", "nonexistent.key")
	want := "nonexistent.key"
	if got != want {
		t.Errorf("T(en, nonexistent.key) = %q, want %q", got, want)
	}
}

func TestT_Interpolation(t *testing.T) {
	got := T("en", "skills.title_topic", "My Topic")
	want := "Skills - My Topic"
	if got != want {
		t.Errorf("T(en, skills.title_topic, My Topic) = %q, want %q", got, want)
	}
}

func TestT_InterpolationPtBR(t *testing.T) {
	got := T("pt-BR", "skills.title_topic", "Meu Tópico")
	want := "Habilidades - Meu Tópico"
	if got != want {
		t.Errorf("T(pt-BR, skills.title_topic, Meu Tópico) = %q, want %q", got, want)
	}
}

func TestSupportedLanguages(t *testing.T) {
	langs := SupportedLanguages()
	if len(langs) < 2 {
		t.Errorf("SupportedLanguages() returned %d languages, want at least 2", len(langs))
	}

	hasEn := false
	hasPtBR := false
	for _, l := range langs {
		if l == "en" {
			hasEn = true
		}
		if l == "pt-BR" {
			hasPtBR = true
		}
	}
	if !hasEn {
		t.Error("SupportedLanguages() missing 'en'")
	}
	if !hasPtBR {
		t.Error("SupportedLanguages() missing 'pt-BR'")
	}
}

func TestIsSupported(t *testing.T) {
	if !IsSupported("en") {
		t.Error("IsSupported(en) = false, want true")
	}
	if !IsSupported("pt-BR") {
		t.Error("IsSupported(pt-BR) = false, want true")
	}
	if IsSupported("fr") {
		t.Error("IsSupported(fr) = true, want false")
	}
}

func TestMatchLanguage(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string
	}{
		{"empty header", "", "en"},
		{"exact match en", "en", "en"},
		{"exact match pt-BR", "pt-BR", "pt-BR"},
		{"prefix match pt", "pt", "pt-BR"},
		{"with quality values", "pt-BR,pt;q=0.9,en-US;q=0.8,en;q=0.7", "pt-BR"},
		{"en-US matches en", "en-US", "en"},
		{"unsupported language falls back", "fr,de;q=0.9", "en"},
		{"unsupported then supported", "fr,pt-BR;q=0.8", "pt-BR"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchLanguage(tt.header)
			if got != tt.want {
				t.Errorf("MatchLanguage(%q) = %q, want %q", tt.header, got, tt.want)
			}
		})
	}
}

func TestDefaultLanguage(t *testing.T) {
	got := DefaultLanguage()
	if got != "en" {
		t.Errorf("DefaultLanguage() = %q, want %q", got, "en")
	}
}

func TestKeyParity(t *testing.T) {
	load()

	enKeys := translations["en"]
	ptBRKeys := translations["pt-BR"]

	// Check all English keys exist in pt-BR
	for key := range enKeys {
		if _, ok := ptBRKeys[key]; !ok {
			t.Errorf("key %q exists in en.json but missing from pt-BR.json", key)
		}
	}

	// Check all pt-BR keys exist in English
	for key := range ptBRKeys {
		if _, ok := enKeys[key]; !ok {
			t.Errorf("key %q exists in pt-BR.json but missing from en.json", key)
		}
	}
}

func TestFuncMap(t *testing.T) {
	fm := FuncMap()
	tFunc, ok := fm["t"]
	if !ok {
		t.Fatal("FuncMap() missing 't' function")
	}

	fn, ok := tFunc.(func(string, string, ...any) string)
	if !ok {
		t.Fatal("FuncMap 't' is not func(string, string, ...any) string")
	}

	got := fn("en", "login.submit")
	if got != "Login" {
		t.Errorf("FuncMap t(en, login.submit) = %q, want %q", got, "Login")
	}
}
