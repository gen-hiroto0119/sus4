// Package icons maps filetree nodes to a Material Design Nerd Font glyph
// + color, in the spirit of nvim-web-devicons.
//
// The glyphs all live in the Material Design Icons range of Nerd Font
// (U+F0000–F1FFF, the nf-md-* namespace), so a single visual family is
// used across the tree. Rendering requires a Nerd Font v3+ in the host
// terminal — without one the cells fall back to "tofu" (□).
package icons

import (
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/gen-hiroto0119/tetra/internal/filetree"
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
			return Icon{Glyph: "\U000f0770", Color: folderColor} // mdi:folder-open
		}
		return Icon{Glyph: "\U000f024b", Color: folderColor} // mdi:folder
	case filetree.NodeTruncated:
		return Icon{Glyph: "\U000f01d8", Color: defaultColor} // mdi:dots-horizontal
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
	return Icon{Glyph: "\U000f0214", Color: defaultColor} // mdi:file
}

var (
	folderColor  = lipgloss.Color("#7ebae4")
	defaultColor = lipgloss.Color("#6d8086")
)

// byExt is keyed by lowercase extension (without leading dot).
// All glyphs are Material Design Icons (nf-md-*).
var byExt = map[string]Icon{
	"go":       {"\U000f07d3", "#00ADD8"}, // mdi:language-go
	"mod":      {"\U000f07d3", "#00ADD8"},
	"sum":      {"\U000f07d3", "#00ADD8"},
	"ts":       {"\U000f06e6", "#3178c6"}, // mdi:language-typescript
	"tsx":      {"\U000f0708", "#3178c6"}, // mdi:react
	"js":       {"\U000f031e", "#f1e05a"}, // mdi:language-javascript
	"jsx":      {"\U000f0708", "#f1e05a"}, // mdi:react
	"mjs":      {"\U000f031e", "#f1e05a"},
	"cjs":      {"\U000f031e", "#f1e05a"},
	"json":     {"\U000f0626", "#cbcb41"}, // mdi:code-json
	"toml":     {"\U000f0219", "#9c4221"}, // mdi:file-document
	"yaml":     {"\U000f0219", "#cb171e"},
	"yml":      {"\U000f0219", "#cb171e"},
	"md":       {"\U000f0354", "#519aba"}, // mdi:language-markdown
	"markdown": {"\U000f0354", "#519aba"},
	"txt":      {"\U000f0219", "#6d8086"}, // mdi:file-document
	"html":     {"\U000f031d", "#e34c26"}, // mdi:language-html5
	"css":      {"\U000f031c", "#563d7c"}, // mdi:language-css3
	"scss":     {"\U000f031c", "#c6538c"},
	"sh":       {"\U000f1183", "#4d5a5e"}, // mdi:bash (or shell)
	"zsh":      {"\U000f1183", "#4d5a5e"},
	"bash":     {"\U000f1183", "#4d5a5e"},
	"fish":     {"\U000f1183", "#4d5a5e"},
	"py":       {"\U000f0320", "#3572A5"}, // mdi:language-python
	"rs":       {"\U000f1617", "#dea584"}, // mdi:language-rust
	"rb":       {"\U000f0d2d", "#701516"}, // mdi:language-ruby
	"c":        {"\U000f0671", "#599eff"}, // mdi:language-c
	"h":        {"\U000f0671", "#a074c4"},
	"cpp":      {"\U000f0672", "#519aba"}, // mdi:language-cpp
	"cc":       {"\U000f0672", "#519aba"},
	"hpp":      {"\U000f0672", "#a074c4"},
	"java":     {"\U000f0b37", "#cc3e44"}, // mdi:language-java
	"kt":       {"\U000f1219", "#F88A02"}, // mdi:language-kotlin
	"swift":    {"\U000f06e5", "#e37933"}, // mdi:language-swift
	"dart":     {"\U000f0174", "#03589C"}, // mdi:code-tags
	"lua":      {"\U000f08b1", "#51a0cf"}, // mdi:language-lua
	"vim":      {"\U000f0174", "#019833"}, // mdi:code-tags
	"lock":     {"\U000f033e", "#bbbbbb"}, // mdi:lock
	"svg":      {"\U000f0721", "#ffb13b"}, // mdi:svg
	"png":      {"\U000f021f", "#a074c4"}, // mdi:file-image
	"jpg":      {"\U000f021f", "#a074c4"},
	"jpeg":     {"\U000f021f", "#a074c4"},
	"gif":      {"\U000f021f", "#a074c4"},
	"webp":     {"\U000f021f", "#a074c4"},
	"pdf":      {"\U000f0226", "#b30b00"}, // mdi:file-pdf-box
	"zip":      {"\U000f05c4", "#eca517"}, // mdi:zip-box
	"tar":      {"\U000f05c4", "#eca517"},
	"gz":       {"\U000f05c4", "#eca517"},
	"tgz":      {"\U000f05c4", "#eca517"},
}

var byName = map[string]Icon{
	"Dockerfile":        {"\U000f0868", "#458ee6"}, // mdi:docker
	"dockerfile":        {"\U000f0868", "#458ee6"},
	".dockerignore":     {"\U000f0868", "#458ee6"},
	"Makefile":          {"\U000f0493", "#6d8086"}, // mdi:cog
	"makefile":          {"\U000f0493", "#6d8086"},
	"LICENSE":           {"\U000f05e6", "#d0bf41"}, // mdi:copyright
	"LICENSE.md":        {"\U000f05e6", "#d0bf41"},
	"LICENSE.txt":       {"\U000f05e6", "#d0bf41"},
	".gitignore":        {"\U000f02a2", "#f54d27"}, // mdi:git
	".gitattributes":    {"\U000f02a2", "#f54d27"},
	".gitmodules":       {"\U000f02a2", "#f54d27"},
	".env":              {"\U000f0306", "#faf743"}, // mdi:key
	".env.local":        {"\U000f0306", "#faf743"},
	"go.mod":            {"\U000f07d3", "#00ADD8"},
	"go.sum":            {"\U000f07d3", "#00ADD8"},
	"package.json":      {"\U000f0399", "#cbcb41"}, // mdi:nodejs
	"package-lock.json": {"\U000f0399", "#cbcb41"},
	"tsconfig.json":     {"\U000f06e6", "#3178c6"},
	"README.md":         {"\U000f14f7", "#519aba"}, // mdi:book-open-variant
	"readme.md":         {"\U000f14f7", "#519aba"},
}
