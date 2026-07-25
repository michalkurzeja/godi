// Package html renders a dependency graph as one self-contained web page.
//
// The page is a Cytoscape canvas driven by the graph model, both inlined. There
// is no CDN, no server and no network access: the file opens from disk and
// works offline, under a content security policy that names the hash of every
// script it carries.
//
// Layout happens in the browser, which is why nothing has to be installed. By
// default that is Graphviz itself, compiled to WebAssembly and inlined, so the
// ranks and the nested scope boxes are the ones Graphviz would draw - but the
// result is a live graph rather than a picture, so nodes can be dragged, the
// graph relaid out, and the wiring filtered and searched.
//
// The layout DOT is built in the browser rather than by graph/dot, and the two
// packages are unrelated. It has to be: Graphviz needs the node sizes, and only
// Cytoscape knows those, because Cytoscape draws the nodes. What it sends is a
// request for geometry - sizes, nesting, edges - with no colour, label or
// legend in it, which is the opposite of what graph/dot produces.
package html

import (
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	stdhtml "html"
	"io"
	"strings"
	"text/template"

	"github.com/michalkurzeja/godi/v2/graph"
	"github.com/michalkurzeja/godi/v2/internal/errorsx"
)

var (
	//go:embed assets/page.html
	pageSrc string
	//go:embed assets/viewer.css
	styleSrc string
	//go:embed assets/viewer.js
	viewerJS string

	// The page is assembled with text/template rather than html/template
	// because the content security policy names the hash of each inline block.
	// That only works if what lands in the file is byte for byte what was
	// hashed, which contextual escaping does not promise. Everything
	// interpolated is either escaped here or produced here.
	pageTmpl = template.Must(template.New("page").Parse(pageSrc))
)

// Encoder writes a graph as an interactive web page.
type Encoder struct {
	cfg config
}

// New returns an HTML encoder.
func New(opts ...Option) *Encoder {
	cfg := config{
		title:  "godi dependency graph",
		theme:  Auto,
		layout: Graphviz,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	return &Encoder{cfg: cfg}
}

func (e *Encoder) Format() graph.Format {
	return graph.Format{Name: "html", Ext: "html", MediaType: "text/html; charset=utf-8"}
}

func (e *Encoder) Encode(g *graph.Graph, w io.Writer) error {
	data, err := json.Marshal(newPayload(g, e.cfg))
	if err != nil {
		return errorsx.Wrap(err, "serialising the graph")
	}

	page := pageData{
		Title:   stdhtml.EscapeString(e.cfg.title),
		Theme:   string(e.cfg.theme),
		Layout:  string(e.cfg.layout),
		Style:   styleSrc,
		Data:    string(data),
		Scripts: e.cfg.scripts(),
	}
	page.CSP = e.policy(page)

	return pageTmpl.Execute(w, page)
}

// pageData is what the skeleton is filled with. Every field is already in its
// final form: the template escapes nothing.
type pageData struct {
	Title   string
	Theme   string
	Layout  string
	CSP     string
	Style   string
	Data    string
	Scripts []string
}

// policy names every piece of inline content by hash, so that nothing else can
// run even if something were to inject into the page.
func (e *Encoder) policy(page pageData) string {
	sources := make([]string, 0, len(page.Scripts)+2)
	if e.cfg.layout == Graphviz {
		// Graphviz is WebAssembly. Instantiating it needs this, and only this:
		// unsafe-eval stays out.
		sources = append(sources, "'wasm-unsafe-eval'")
	}
	sources = append(sources, "'"+hash(page.Data)+"'")
	for _, script := range page.Scripts {
		sources = append(sources, "'"+hash(script)+"'")
	}

	return strings.Join([]string{
		"default-src 'none'",
		// Cytoscape renders to a canvas it sizes with style attributes, which a
		// hash cannot cover: naming one here would make the browser ignore
		// unsafe-inline and the page would not draw. Scripts stay hash-locked,
		// which is where the risk actually is.
		"style-src 'unsafe-inline'",
		"script-src " + strings.Join(sources, " "),
		"base-uri 'none'",
		"form-action 'none'",
	}, "; ")
}

// hash is the content security policy's name for a piece of inline content.
func hash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return "sha256-" + base64.StdEncoding.EncodeToString(sum[:])
}
