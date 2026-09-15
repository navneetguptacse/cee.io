package languages

import (
	"sort"
	"sync"
)

// Language IDs
const (
	LangBash       = 46
	LangC          = 50
	LangCPP        = 54
	LangGo         = 60
	LangJava       = 62
	LangJavaScript = 63
	LangPython     = 71
	LangRust       = 73
	LangTypeScript = 74
	LangMultiFile  = 89
)

var (
	registryLock sync.RWMutex
	languages    = map[int]*Language{}
	activeList   = []*Language{}
)

func init() {
	langs := []*Language{
		{
			ID:         LangBash,
			Name:       "Bash (5.0.17)",
			IsArchived: false,
			SourceFile: "script.sh",
			CompileCmd: "",
			RunCmd:     "bash script.sh",
			Image:      "cee/bash:5",
			MinMemory:  64000,
		},
		{
			ID:         LangC,
			Name:       "C (GCC 9.2.0)",
			IsArchived: false,
			SourceFile: "main.c",
			CompileCmd: "gcc -O2 -o a.out main.c",
			RunCmd:     "./a.out",
			Image:      "cee/gcc:9",
			MinMemory:  64000,
		},
		{
			ID:         LangCPP,
			Name:       "C++ (GCC 9.2.0)",
			IsArchived: false,
			SourceFile: "main.cpp",
			CompileCmd: "g++ -O2 -std=c++17 -o a.out main.cpp",
			RunCmd:     "./a.out",
			Image:      "cee/gcc:9",
			MinMemory:  128000,
		},
		{
			ID:         LangGo,
			Name:       "Go (1.22.0)",
			IsArchived: false,
			SourceFile: "main.go",
			CompileCmd: "go build -o a.out main.go",
			RunCmd:     "./a.out",
			Image:      "cee/golang:1.22",
			MinMemory:  256000,
		},
		{
			ID:         LangJava,
			Name:       "Java (OpenJDK 17)",
			IsArchived: false,
			SourceFile: "Main.java",
			CompileCmd: "javac -cp .:/usr/local/lib/java/* *.java",
			RunCmd:     "java -cp .:/usr/local/lib/java/* Main",
			Image:      "cee/java:17",
			MinMemory:  256000,
		},
		{
			ID:         LangJavaScript,
			Name:       "JavaScript (Node.js 18.15.0)",
			IsArchived: false,
			SourceFile: "script.js",
			CompileCmd: "",
			RunCmd:     "node script.js",
			Image:      "cee/node:18",
			MinMemory:  128000,
		},
		{
			ID:         LangPython,
			Name:       "Python (3.8.10)",
			IsArchived: false,
			SourceFile: "main.py",
			CompileCmd: "",
			RunCmd:     "python3 main.py",
			Image:      "cee/python:3.8",
			MinMemory:  64000,
		},
		{
			ID:         LangRust,
			Name:       "Rust (1.75.0)",
			IsArchived: false,
			SourceFile: "main.rs",
			CompileCmd: "rustc -O -o a.out main.rs",
			RunCmd:     "./a.out",
			Image:      "cee/rust:latest",
			MinMemory:  256000,
		},
		{
			ID:         LangTypeScript,
			Name:       "TypeScript (5.0.3)",
			IsArchived: false,
			SourceFile: "ts-main.ts",
			CompileCmd: "tsc ts-main.ts --outDir . --esModuleInterop --skipLibCheck",
			RunCmd:     "node ts-main.js",
			Image:      "cee/typescript:5",
			MinMemory:  512000, // tsc needs memory for typechecking
		},
		{
			ID:         LangMultiFile,
			Name:       "Multi-file program",
			IsArchived: false,
			SourceFile: "",
			CompileCmd: "",
			RunCmd:     "",
			Image:      "cee/multi:latest",
			MinMemory:  128000,
		},
	}

	for _, l := range langs {
		languages[l.ID] = l
		activeList = append(activeList, l)
	}

	aliases := map[int]int{
		92:  LangPython,
		93:  LangJavaScript,
		94:  LangTypeScript,
		95:  LangGo,
		102: LangJavaScript,
	}

	for aliasID, targetID := range aliases {
		if target, ok := languages[targetID]; ok {
			languages[aliasID] = target
		}
	}
}

// GetLanguageByID returns the language by its ID or nil.
func GetLanguageByID(id int) *Language {
	registryLock.RLock()
	defer registryLock.RUnlock()
	return languages[id]
}

// IsValidLanguageID checks if an ID is supported.
func IsValidLanguageID(id int) bool {
	registryLock.RLock()
	defer registryLock.RUnlock()
	_, ok := languages[id]
	return ok
}

// GetActiveLanguages returns all non-archived canonical languages sorted by ID.
func GetActiveLanguages() []*Language {
	registryLock.RLock()
	defer registryLock.RUnlock()
	res := make([]*Language, len(activeList))
	copy(res, activeList)
	sort.Slice(res, func(i, j int) bool {
		return res[i].ID < res[j].ID
	})
	return res
}

// GetAllLanguages returns all active and archived languages.
func GetAllLanguages() []*Language {
	return GetActiveLanguages()
}
