package tests

import (
	"testing"

	"cee.io/pkg/languages"
)

func TestLanguagesRegistry(t *testing.T) {
	// Canonical languages
	expectedIDs := []int{
		languages.LangBash,
		languages.LangC,
		languages.LangCPP,
		languages.LangGo,
		languages.LangJava,
		languages.LangJavaScript,
		languages.LangPython,
		languages.LangRust,
		languages.LangTypeScript,
		languages.LangMultiFile,
	}

	for _, id := range expectedIDs {
		lang := languages.GetLanguageByID(id)
		if lang == nil {
			t.Errorf("expected language ID %d to be present", id)
			continue
		}
		if !languages.IsValidLanguageID(id) {
			t.Errorf("expected IsValidLanguageID(%d) to be true", id)
		}
	}

	// RapidAPI aliases
	aliasTests := map[int]int{
		92:  languages.LangPython,
		93:  languages.LangJavaScript,
		94:  languages.LangTypeScript,
		95:  languages.LangGo,
		102: languages.LangJavaScript,
	}

	for alias, canonical := range aliasTests {
		l := languages.GetLanguageByID(alias)
		if l == nil {
			t.Errorf("expected alias %d to exist", alias)
			continue
		}
		if l.ID != canonical {
			t.Errorf("alias %d resolved to ID %d, expected %d", alias, l.ID, canonical)
		}
	}
}

func TestJudge0Statuses(t *testing.T) {
	all := languages.GetAllStatuses()
	if len(all) != 14 {
		t.Errorf("expected 14 statuses, got %d", len(all))
	}

	s3 := languages.GetStatusByID(languages.StatusAccepted)
	if s3.Description != "Accepted" {
		t.Errorf("expected Status 3 to be 'Accepted', got %s", s3.Description)
	}

	s5 := languages.GetStatusByID(languages.StatusTimeLimitExceeded)
	if s5.Description != "Time Limit Exceeded" {
		t.Errorf("expected Status 5 to be 'Time Limit Exceeded', got %s", s5.Description)
	}
}

func TestHostLanguageAvailability(t *testing.T) {
	// Bash should be available on all Unix/macOS test machines
	bashLang := languages.GetLanguageByID(languages.LangBash)
	if bashLang == nil {
		t.Fatalf("expected Bash language to exist")
	}
	ok, missing := bashLang.IsHostAvailable()
	if !ok {
		t.Errorf("expected bash to be available on host, missing: %s", missing)
	}

	// Nil language test
	var nilLang *languages.Language
	ok, _ = nilLang.IsHostAvailable()
	if ok {
		t.Errorf("expected nil language to return false")
	}
}

