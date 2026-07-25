package html

import (
	"slices"
	"strings"
)

// Link templates for the editors people tend to use, to save writing one out.
// Pass one to SourceLink:
//
//	html.New(html.SourceLink(html.VSCode))
//
// They are templates like any other, so an editor that is not here, or one that
// wants different arguments, is a string away:
//
//	html.New(html.SourceLink("myeditor://open?at={file}#{line}"))
//
// Two families, because that is how the editors split. The VS Code lineage
// takes the path in the URL body; the rest take it as a query parameter.
//
// Whether a link opens anything depends on the editor having registered its
// scheme with the operating system, which most do on install. The JetBrains and
// Sublime ones are the least certain: JetBrains publishes a second, project
// relative form for its Toolbox, and Sublime's scheme comes from a plugin
// rather than the editor itself.
const (
	VSCode         = "vscode://file{file}:{line}"
	VSCodeInsiders = "vscode-insiders://file{file}:{line}"
	VSCodium       = "vscodium://file{file}:{line}"
	Cursor         = "cursor://file{file}:{line}"
	Windsurf       = "windsurf://file{file}:{line}"
	Zed            = "zed://file{file}:{line}"

	GoLand   = "goland://open?file={file}&line={line}"
	IntelliJ = "idea://open?file={file}&line={line}"
	TextMate = "txmt://open?url=file://{file}&line={line}"
	BBEdit   = "x-bbedit://open?url=file://{file}&line={line}"
	Sublime  = "subl://open?url=file://{file}&line={line}"
)

// editors is the same set again, by name, for callers that take the editor as
// a string rather than as code.
var editors = map[string]string{
	"vscode":          VSCode,
	"vscode-insiders": VSCodeInsiders,
	"vscodium":        VSCodium,
	"cursor":          Cursor,
	"windsurf":        Windsurf,
	"zed":             Zed,
	"goland":          GoLand,
	"intellij":        IntelliJ,
	"idea":            IntelliJ,
	"textmate":        TextMate,
	"bbedit":          BBEdit,
	"sublime":         Sublime,
}

// EditorLink is the link template for a named editor, for anything that takes
// the editor as a string: a command line flag, a config file, an environment
// variable. Names are the lowercase ones in Editors.
//
// It reports false for a name it does not know, so a caller can tell a typo
// from a template of the reader's own and say so.
func EditorLink(name string) (string, bool) {
	template, ok := editors[strings.ToLower(strings.TrimSpace(name))]
	return template, ok
}

// Editors lists the names EditorLink accepts, sorted, for putting in a usage
// message.
func Editors() []string {
	names := make([]string, 0, len(editors))
	for name := range editors {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}
