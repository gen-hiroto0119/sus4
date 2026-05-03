// Package icons maps filetree nodes to a Nerd Font glyph + color, in the
// spirit of nvim-web-devicons.
//
// The glyphs live in the Nerd Font Private Use Area (U+E000–F8FF), so
// rendering requires a Nerd Font in the host terminal. With a regular
// font the cells fall back to "tofu" (□).
package icons

import (
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/gen-hiroto0119/sus4/internal/filetree"
)

type Icon struct {
	Glyph string
	Color lipgloss.Color
}

// For returns the icon for node n. expanded only matters for directories.
// Lookup precedence: exact filename → lowercase filename → extension →
// default. NodeTruncated returns a generic ellipsis-ish glyph.
func For(n filetree.Node, expanded bool) Icon {
	switch n.Kind {
	case filetree.NodeDir:
		if expanded {
			return Icon{Glyph: "", Color: folderColor} //
		}
		return Icon{Glyph: "", Color: folderColor} //
	case filetree.NodeTruncated:
		return Icon{Glyph: "", Color: defaultColor} //
	}

	if ic, ok := byName[n.Name]; ok {
		return ic
	}
	if ic, ok := byName[strings.ToLower(n.Name)]; ok {
		return ic
	}
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(n.Name), "."))
	if ic, ok := byExt[ext]; ok {
		return ic
	}
	return Icon{Glyph: "", Color: defaultColor} //
}

var (
	folderColor  = lipgloss.Color("#7ebae4")
	defaultColor = lipgloss.Color("#6d8086")
)

// byExt is keyed by lowercase extension (without leading dot).
// Glyph values follow the nvim-web-devicons defaults where practical.
var byExt = map[string]Icon{
	"go":       {"", "#00ADD8"},
	"mod":      {"", "#00ADD8"},
	"sum":      {"", "#00ADD8"},
	"ts":       {"", "#3178c6"},
	"tsx":      {"", "#3178c6"},
	"js":       {"", "#f1e05a"},
	"jsx":      {"", "#f1e05a"},
	"mjs":      {"", "#f1e05a"},
	"cjs":      {"", "#f1e05a"},
	"json":     {"", "#cbcb41"},
	"toml":     {"", "#9c4221"},
	"yaml":     {"", "#cb171e"},
	"yml":      {"", "#cb171e"},
	"md":       {"", "#519aba"},
	"markdown": {"", "#519aba"},
	"txt":      {"", "#6d8086"},
	"html":     {"", "#e34c26"},
	"css":      {"", "#563d7c"},
	"scss":     {"", "#c6538c"},
	"sh":       {"", "#4d5a5e"},
	"zsh":      {"", "#4d5a5e"},
	"bash":     {"", "#4d5a5e"},
	"fish":     {"", "#4d5a5e"},
	"py":       {"", "#3572A5"},
	"rs":       {"", "#dea584"},
	"rb":       {"", "#701516"},
	"c":        {"", "#599eff"},
	"h":        {"", "#a074c4"},
	"cpp":      {"", "#519aba"},
	"cc":       {"", "#519aba"},
	"hpp":      {"", "#a074c4"},
	"java":     {"", "#cc3e44"},
	"kt":       {"", "#F88A02"},
	"swift":    {"", "#e37933"},
	"dart":     {"", "#03589C"},
	"lua":      {"", "#51a0cf"},
	"vim":      {"", "#019833"},
	"lock":     {"", "#bbbbbb"},
	"svg":      {"ﰟ", "#ffb13b"},
	"png":      {"", "#a074c4"},
	"jpg":      {"", "#a074c4"},
	"jpeg":     {"", "#a074c4"},
	"gif":      {"", "#a074c4"},
	"webp":     {"", "#a074c4"},
	"pdf":      {"", "#b30b00"},
	"zip":      {"", "#eca517"},
	"tar":      {"", "#eca517"},
	"gz":       {"", "#eca517"},
	"tgz":      {"", "#eca517"},
}

var byName = map[string]Icon{
	"Dockerfile":        {"", "#458ee6"},
	"dockerfile":        {"", "#458ee6"},
	".dockerignore":     {"", "#458ee6"},
	"Makefile":          {"", "#6d8086"},
	"makefile":          {"", "#6d8086"},
	"LICENSE":           {"", "#d0bf41"},
	"LICENSE.md":        {"", "#d0bf41"},
	"LICENSE.txt":       {"", "#d0bf41"},
	".gitignore":        {"", "#f54d27"},
	".gitattributes":    {"", "#f54d27"},
	".gitmodules":       {"", "#f54d27"},
	".env":              {"", "#faf743"},
	".env.local":        {"", "#faf743"},
	"go.mod":            {"", "#00ADD8"},
	"go.sum":            {"", "#00ADD8"},
	"package.json":      {"", "#cbcb41"},
	"package-lock.json": {"", "#cbcb41"},
	"tsconfig.json":     {"", "#3178c6"},
	"README.md":         {"", "#519aba"},
	"readme.md":         {"", "#519aba"},
}
