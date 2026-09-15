package languages

// Language defines the configuration and execution parameters for a programming language.
type Language struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	IsArchived bool   `json:"is_archived"`
	SourceFile string `json:"source_file,omitempty"`
	CompileCmd string `json:"compile_cmd,omitempty"`
	RunCmd     string `json:"run_cmd,omitempty"`
	Image      string `json:"image,omitempty"`
	MinMemory  int    `json:"min_memory,omitempty"` // in KB
}
