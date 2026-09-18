package main

import (
	"path/filepath"
	"strconv"
	"strings"

	"cee.io/pkg/languages"
)

func detectLanguageID(path string) int {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".py":
		return languages.LangPython
	case ".js", ".mjs":
		return languages.LangJavaScript
	case ".ts":
		return languages.LangTypeScript
	case ".go":
		return languages.LangGo
	case ".cpp", ".cc", ".cxx":
		return languages.LangCPP
	case ".c":
		return languages.LangC
	case ".java":
		return languages.LangJava
	case ".rs":
		return languages.LangRust
	case ".sh":
		return languages.LangBash
	case ".zip":
		return languages.LangMultiFile
	default:
		return 0
	}
}

func parseLanguage(s string) *languages.Language {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return nil
	}

	if id, err := strconv.Atoi(s); err == nil {
		return languages.GetLanguageByID(id)
	}

	switch s {
	case "py", "python", "python3":
		return languages.GetLanguageByID(languages.LangPython)
	case "js", "javascript", "node", "nodejs":
		return languages.GetLanguageByID(languages.LangJavaScript)
	case "ts", "typescript":
		return languages.GetLanguageByID(languages.LangTypeScript)
	case "go", "golang":
		return languages.GetLanguageByID(languages.LangGo)
	case "cpp", "c++", "g++":
		return languages.GetLanguageByID(languages.LangCPP)
	case "c", "gcc":
		return languages.GetLanguageByID(languages.LangC)
	case "java", "openjdk":
		return languages.GetLanguageByID(languages.LangJava)
	case "rs", "rust":
		return languages.GetLanguageByID(languages.LangRust)
	case "sh", "bash", "shell":
		return languages.GetLanguageByID(languages.LangBash)
	default:
		for _, l := range languages.GetAllLanguages() {
			if strings.Contains(strings.ToLower(l.Name), s) {
				return l
			}
		}
		return nil
	}
}

func looksLikeFilePath(s string) bool {
	if strings.ContainsAny(s, "/\\") {
		return true
	}
	return filepath.Ext(s) != ""
}
