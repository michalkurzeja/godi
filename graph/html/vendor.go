package html

import _ "embed"

// The viewer is built on third-party code, vendored under assets/vendor and
// inlined verbatim so that the page needs no network. See that directory's
// NOTICE.md for versions, licences and how to update them.
var (
	//go:embed assets/vendor/cytoscape.min.js
	cytoscapeJS string
	//go:embed assets/vendor/viz-global.js
	vizJS string
	//go:embed assets/vendor/dagre.min.js
	dagreJS string
	//go:embed assets/vendor/cytoscape-dagre.min.js
	cytoscapeDagreJS string
)

// Inlining a bundle redistributes it, and the MIT licence asks that the notice
// travels with the copy. All of them carry their own header except dagre, so its
// header is prepended here.
const dagreNotice = "/*! dagre 3.0.0 | (c) 2012-2014 Chris Pettitt | MIT" +
	" | https://github.com/dagrejs/dagre */\n"

// scripts returns the bundles the page needs, in load order: the renderer, then
// whichever layout engine was asked for, then the viewer itself.
//
// Only the chosen engine is inlined. Graphviz is much the larger of the two, so a
// page laid out by dagre is a fraction of the size.
func (cfg config) scripts() []string {
	out := []string{cytoscapeJS}

	if cfg.layout == Dagre {
		// cytoscape-dagre registers itself with the global cytoscape, so the order
		// matters: renderer first, then the engine, then the adapter.
		out = append(out, dagreNotice+dagreJS, cytoscapeDagreJS)
	} else {
		out = append(out, vizJS)
	}

	return append(out, viewerJS)
}

// credits is what the page shows when the reader asks what it is built on.
type credit struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Licence string `json:"licence"`
	URL     string `json:"url"`
}

func (cfg config) credits() []credit {
	out := []credit{
		{"cytoscape", "3.34.0", "MIT", "https://github.com/cytoscape/cytoscape.js"},
	}

	if cfg.layout == Dagre {
		return append(out,
			credit{"dagre", "3.0.0", "MIT", "https://github.com/dagrejs/dagre"},
			credit{"cytoscape-dagre", "4.0.0", "MIT", "https://github.com/cytoscape/cytoscape.js-dagre"},
		)
	}
	return append(out,
		credit{"viz.js", "3.28.0", "MIT", "https://github.com/mdaines/viz-js"},
		credit{"graphviz", "15.0.0", "EPL-2.0", "https://graphviz.org"},
	)
}
