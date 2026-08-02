package dot

const fontName = "Helvetica"

// RankDirection is the direction the graph flows in.
type RankDirection string

const (
	LR RankDirection = "LR" // Left to right. Suits the wide, shallow graphs wiring tends to produce.
	TB RankDirection = "TB" // Top to bottom.
)

// ThemeName selects the colour palette.
type ThemeName string

const (
	// Light draws on an explicit white background. A single Graphviz SVG cannot
	// be readable on both backgrounds, because the text colour is baked in.
	Light ThemeName = "light"
	Dark  ThemeName = "dark"
)

// PortMode controls whether each argument gets its own row, with edges landing
// on the exact row they feed.
type PortMode int

const (
	// PortsAuto draws argument rows while the graph is small enough to read them.
	PortsAuto PortMode = iota
	PortsOn
	PortsOff
)

func (m PortMode) enabled(nodes int) bool {
	switch m {
	case PortsOn:
		return true
	case PortsOff:
		return false
	case PortsAuto:
		return nodes <= 150
	}
	return true
}

type config struct {
	rankDir  RankDirection
	theme    ThemeName
	ports    PortMode
	legend   bool
	maxLabel int
}

// Option configures the DOT encoder.
type Option func(*config)

// RankDir sets the direction the graph flows in. Defaults to LR.
func RankDir(dir RankDirection) Option {
	return func(cfg *config) { cfg.rankDir = dir }
}

// Theme sets the colour palette. Defaults to Light.
func Theme(name ThemeName) Option {
	return func(cfg *config) { cfg.theme = name }
}

// Ports controls the per-argument rows. Defaults to PortsAuto.
func Ports(mode PortMode) Option {
	return func(cfg *config) { cfg.ports = mode }
}

// Legend draws the key explaining the line styles and arrowheads. On by default.
func Legend(on bool) Option {
	return func(cfg *config) { cfg.legend = on }
}

// MaxLabel truncates type names in argument rows to this many characters.
func MaxLabel(chars int) Option {
	return func(cfg *config) { cfg.maxLabel = chars }
}

type palette struct {
	background    string
	nodeFill      string
	rootFill      string
	nodeBorder    string
	text          string
	muted         string
	clusterBorder string
	clusterTint   string

	manual    string
	autowired string
	pass      string
	warn      string
}

func (p palette) clusterFill(depth int) string {
	// Nested scopes get progressively more tint, so depth reads at a glance. A
	// negative depth cannot come from a built container, but a hand-edited or
	// corrupted graph file is not validated against, so it is clamped rather
	// than trusted.
	alphas := []string{"06", "0a", "10", "16"}
	return p.clusterTint + alphas[min(max(depth, 0), len(alphas)-1)]
}

func (n ThemeName) palette() palette {
	if n == Dark {
		return palette{
			background:    "#0d1117",
			nodeFill:      "#161b22",
			rootFill:      "#1b3557",
			nodeBorder:    "#6e7681",
			text:          "#e6edf3",
			muted:         "#9198a1",
			clusterBorder: "#30363d",
			clusterTint:   "#ffffff",
			manual:        "#e6edf3",
			autowired:     "#2dd4bf",
			pass:          "#c297ff",
			warn:          "#ff7b72",
		}
	}
	return palette{
		background:    "#ffffff",
		nodeFill:      "#f4f5f7",
		rootFill:      "#dbe9fc",
		nodeBorder:    "#8a8f98",
		text:          "#1f2328",
		muted:         "#57606a",
		clusterBorder: "#c9ced6",
		clusterTint:   "#000000",
		manual:        "#1f2328",
		autowired:     "#0f766e",
		pass:          "#8250df",
		warn:          "#cf222e",
	}
}
