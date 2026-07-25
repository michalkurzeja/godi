package html

// ThemeName selects the colour scheme.
type ThemeName string

const (
	// Auto follows the reader's system setting, and switches live when it
	// changes. Colours are applied at render time rather than baked into a
	// drawing, so this costs nothing.
	Auto ThemeName = "auto"
	// Light and Dark commit the page to one scheme.
	Light ThemeName = "light"
	Dark  ThemeName = "dark"
)

// LayoutEngine is the algorithm that decides where the nodes go. Whichever is
// chosen, the page is drawn and driven by Cytoscape: the engine only supplies
// positions, and the reader can still drag a node anywhere afterwards.
type LayoutEngine string

const (
	// Graphviz is Graphviz itself, compiled to WebAssembly and inlined. It runs
	// in the browser, so nothing has to be installed, and it lays out a
	// dependency graph better than anything else available: proper layered
	// ranks, and scopes that come out as real nested boxes.
	//
	// It costs about 1.3MB of page. That is the default because a graph you
	// cannot read is worth less than a file you have to wait a moment for.
	Graphviz LayoutEngine = "graphviz"
	// Dagre is a small layered layout in plain JavaScript, about 85KB. Choose it
	// for a much smaller page, or to keep the file free of Graphviz's EPL-2.0
	// object code. Its ranks are plainer and it handles nested scopes less
	// tidily.
	Dagre LayoutEngine = "dagre"
)

type config struct {
	title      string
	theme      ThemeName
	layout     LayoutEngine
	sourceLink string
}

// Option configures the HTML encoder.
type Option func(*config)

// Title names the page. Defaults to "godi dependency graph".
func Title(title string) Option {
	return func(cfg *config) { cfg.title = title }
}

// Theme sets the colour scheme. Defaults to Auto.
func Theme(name ThemeName) Option {
	return func(cfg *config) { cfg.theme = name }
}

// Layout picks the engine that places the nodes. Defaults to Graphviz.
func Layout(engine LayoutEngine) Option {
	return func(cfg *config) { cfg.layout = engine }
}

// SourceLink makes the locations in the detail panel clickable, by giving the
// page a template to build a URL from. Without it they are plain text, because
// a file:// link to a .go file only offers to download it.
//
// Three placeholders are substituted: {file} for the absolute path, {rel} for
// the path relative to the graph's source root, and {line}.
//
//	html.SourceLink(html.VSCode)                      // a preset
//	html.SourceLink("myeditor://open?at={file}#{line}") // or your own
//
// See editors.go for the presets, and EditorLink to look one up by name.
//
// A binary built with -trimpath records module-relative paths, which no editor
// can open. The links are only as good as the paths the build left behind.
func SourceLink(template string) Option {
	return func(cfg *config) { cfg.sourceLink = template }
}
