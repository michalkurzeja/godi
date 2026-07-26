package graph

import (
	"encoding/json"
	"fmt"
	"io"
	"runtime/debug"
	"time"

	"github.com/michalkurzeja/godi/v2/internal/errorsx"
)

// modulePath is godi's own module, as a build records it.
const modulePath = "github.com/michalkurzeja/godi/v2"

// Metadata describes the file a graph was written to, rather than the graph
// itself. It is what tells a reader whether it understands what follows.
type Metadata struct {
	// Schema is the shape the graph was written against. A reader compares it
	// with its own Schema.
	Schema string `json:"schema"`
	// WrittenAt is when the file was written, which is how you tell one failure
	// graph from another once a few have collected in a temporary directory.
	WrittenAt time.Time `json:"writtenAt,omitzero"`
	// GodiVersion is the version of godi that wrote the graph, where the build
	// records one. It is empty under `go run` and in tests.
	GodiVersion string `json:"godiVersion,omitzero"`
}

// envelope is the file: what this is, and then the graph.
type envelope struct {
	Metadata Metadata `json:"metadata"`
	Graph    *Graph   `json:"graph"`
}

// WriteJSON writes the graph as JSON, wrapped in an envelope carrying the schema
// it was written against.
//
// This is the interchange format. It is how a container that failed to build
// hands its graph to something that can draw it, without the library having to
// depend on a renderer to do so.
func (g *Graph) WriteJSON(w io.Writer) error {
	return g.writeJSON(w, "")
}

func (g *Graph) writeJSON(w io.Writer, indent string) error {
	if g == nil {
		return fmt.Errorf("graph: cannot write a nil graph")
	}

	schema := g.Schema
	if schema == "" {
		schema = Schema
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", indent)

	err := enc.Encode(envelope{
		Metadata: Metadata{
			Schema:      schema,
			WrittenAt:   time.Now().UTC(),
			GodiVersion: godiVersion(),
		},
		Graph: g,
	})
	if err != nil {
		return errorsx.Wrap(err, "graph: writing json")
	}
	return nil
}

// ReadJSON reads a graph written by WriteJSON, along with the metadata of the
// file it came from.
//
// A schema it does not recognise is a warning rather than a failure. The model
// grows by adding fields, so a reader of one version usually still makes sense
// of a file from another, and half a graph beats none when you are looking at a
// build that already went wrong. The warning is recorded as a Diagnostic, so it
// travels with the graph into whatever renders it.
func ReadJSON(r io.Reader) (*Graph, Metadata, error) {
	var env envelope

	err := json.NewDecoder(r).Decode(&env)
	if err != nil {
		return nil, Metadata{}, errorsx.Wrap(err, "graph: reading json")
	}
	if env.Graph == nil {
		return nil, env.Metadata, fmt.Errorf("graph: the file carries no graph")
	}

	g := env.Graph
	g.Schema = env.Metadata.Schema // The file's own account of itself, not ours.

	if g.Schema != Schema {
		g.GraphDiagnostics = append(g.GraphDiagnostics, &Diagnostic{
			Severity: SeverityWarning,
			Message:  schemaMismatch(g.Schema),
		})
	}

	return g, env.Metadata, nil
}

func schemaMismatch(found string) string {
	if found == "" {
		return fmt.Sprintf("this graph names no schema and was read as %s; some of it may be wrong", Schema)
	}
	return fmt.Sprintf("this graph was written as %s and read as %s; some of it may be wrong", found, Schema)
}

// JSONOption configures the JSON encoder.
type JSONOption func(*jsonConfig)

type jsonConfig struct {
	indent string
}

// Indent writes each level of nesting with the given string. Defaults to none:
// the format is written far more often than it is read by eye.
func Indent(indent string) JSONOption {
	return func(cfg *jsonConfig) { cfg.indent = indent }
}

// JSON returns an encoder for the interchange format, so that it can be reached
// the same way as any other format. It writes what WriteJSON writes.
func JSON(opts ...JSONOption) Encoder {
	var cfg jsonConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	return &jsonEncoder{cfg: cfg}
}

type jsonEncoder struct {
	cfg jsonConfig
}

func (e *jsonEncoder) Format() Format {
	return Format{Name: "json", Ext: "json", MediaType: "application/json"}
}

func (e *jsonEncoder) Encode(g *Graph, w io.Writer) error {
	return g.writeJSON(w, e.cfg.indent)
}

func godiVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	if info.Main.Path == modulePath {
		return info.Main.Version
	}
	for _, dep := range info.Deps {
		if dep.Path == modulePath {
			return dep.Version
		}
	}
	return ""
}
