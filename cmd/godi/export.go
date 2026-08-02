package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/michalkurzeja/godi/v2/graph"
	"github.com/michalkurzeja/godi/v2/graph/dot"
	"github.com/michalkurzeja/godi/v2/graph/html"
	"github.com/michalkurzeja/godi/v2/graph/json"
	"github.com/michalkurzeja/godi/v2/graph/text"
)

func newExportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Write a graph out in a given format",
		Long: `Write a graph out in a given format.

The graph is read from the named file, or from standard input when no file is
named. The result goes to standard output unless --output says otherwise.`,
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return fmt.Errorf("export: a format is required: %s", strings.Join([]string{"text", "dot", "html", "json"}, ", "))
		},
	}

	cmd.PersistentFlags().StringP("output", "o", "-", "write to this file instead of standard output")
	cmd.AddCommand(newExportTextCmd(), newExportDotCmd(), newExportHTMLCmd(), newExportJSONCmd())

	return cmd
}

// export is the whole of every export command once its encoder is built.
func export(cmd *cobra.Command, args []string, enc graph.Encoder) error {
	g, err := readGraph(cmd, args)
	if err != nil {
		return err
	}

	w, closeOut, err := openOutput(cmd)
	if err != nil {
		return err
	}
	defer closeOut()

	return g.Encode(w, enc)
}

func newExportTextCmd() *cobra.Command {
	var (
		noLocations bool
		maxType     int
	)

	cmd := &cobra.Command{
		Use:   "text [file]",
		Short: "Write the graph as an indented outline",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := []text.Option{text.MaxType(maxType)}
			if noLocations {
				opts = append(opts, text.WithoutLocations())
			}
			return export(cmd, args, text.New(opts...))
		},
	}

	cmd.Flags().BoolVar(&noLocations, "no-locations", false, "leave out where each definition was registered and declared")
	cmd.Flags().IntVar(&maxType, "max-type", 48, "truncate type names to this many runes, or 0 for no limit")
	completeGraphFiles(cmd)

	return cmd
}

func newExportDotCmd() *cobra.Command {
	var opts dotOptions

	cmd := &cobra.Command{
		Use:   "dot [file]",
		Short: "Write the graph as Graphviz DOT",
		Long: `Write the graph as Graphviz DOT.

	godi export dot graph.json | dot -Tsvg -o graph.svg`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			enc, err := opts.encoder()
			if err != nil {
				return err
			}
			return export(cmd, args, enc)
		},
	}

	opts.register(cmd)
	completeGraphFiles(cmd)

	return cmd
}

func newExportHTMLCmd() *cobra.Command {
	var opts htmlOptions

	cmd := &cobra.Command{
		Use:   "html [file]",
		Short: "Write the graph as a self-contained page",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			enc, err := opts.encoder()
			if err != nil {
				return err
			}
			return export(cmd, args, enc)
		},
	}

	opts.register(cmd)
	completeGraphFiles(cmd)

	return cmd
}

func newExportJSONCmd() *cobra.Command {
	var indent string

	cmd := &cobra.Command{
		Use:   "json [file]",
		Short: "Write the graph back out as JSON",
		Long: `Write the graph back out as JSON.

This is the format it arrived in, so the command is for reading it by eye or
normalising it before a diff:

	godi export json --indent '  ' graph.json`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return export(cmd, args, json.New(json.Indent(indent)))
		},
	}

	cmd.Flags().StringVar(&indent, "indent", "", "indent each level of nesting with this string")
	completeGraphFiles(cmd)

	return cmd
}

type dotOptions struct {
	rankDir  string
	theme    string
	ports    string
	legend   bool
	maxLabel int
}

func (o *dotOptions) register(cmd *cobra.Command) {
	cmd.Flags().StringVar(&o.rankDir, "rankdir", "LR", "direction the graph flows in: "+orList(rankDirs))
	cmd.Flags().StringVar(&o.theme, "theme", "light", "colour palette: "+orList(dotThemes))
	cmd.Flags().StringVar(&o.ports, "ports", "auto", "per-argument rows: "+orList(portModes))
	cmd.Flags().BoolVar(&o.legend, "legend", true, "draw the key explaining the line styles")
	cmd.Flags().IntVar(&o.maxLabel, "max-label", 44, "truncate type names in argument rows to this many characters")

	completeWith(cmd, "rankdir", rankDirs)
	completeWith(cmd, "theme", dotThemes)
	completeWith(cmd, "ports", portModes)
}

func (o *dotOptions) encoder() (graph.Encoder, error) {
	dir, err := o.parseRankDir()
	if err != nil {
		return nil, err
	}
	palette, err := o.parseTheme()
	if err != nil {
		return nil, err
	}
	mode, err := o.parsePorts()
	if err != nil {
		return nil, err
	}

	return dot.New(
		dot.RankDir(dir),
		dot.Theme(palette),
		dot.Ports(mode),
		dot.Legend(o.legend),
		dot.MaxLabel(o.maxLabel),
	), nil
}

// htmlOptions is shared by export html and view, which draw the same page.
type htmlOptions struct {
	title  string
	theme  string
	layout string
	link   string
}

func (o *htmlOptions) register(cmd *cobra.Command) {
	cmd.Flags().StringVar(&o.title, "title", "godi dependency graph", "name the page")
	cmd.Flags().StringVar(&o.theme, "theme", "auto", "colour scheme: "+orList(htmlThemes))
	cmd.Flags().StringVar(&o.layout, "layout", "graphviz", "engine that places the nodes: "+orList(layouts))
	cmd.Flags().StringVar(&o.link, "link", "",
		"make source locations clickable, by editor name ("+strings.Join(html.Editors(), ", ")+") or a link template")

	completeWith(cmd, "theme", htmlThemes)
	completeWith(cmd, "layout", layouts)
	completeWith(cmd, "link", html.Editors())
}

func (o *htmlOptions) encoder() (graph.Encoder, error) {
	theme, err := o.parseTheme()
	if err != nil {
		return nil, err
	}
	layout, err := o.parseLayout()
	if err != nil {
		return nil, err
	}

	opts := []html.Option{html.Title(o.title), html.Theme(theme), html.Layout(layout)}
	if o.link != "" {
		// An unknown name is taken as a template of the reader's own, which is
		// what SourceLink documents. Only a name that looks like neither is worth
		// reporting.
		template, ok := html.EditorLink(o.link)
		if !ok {
			if !strings.Contains(o.link, "{file}") {
				return nil, fmt.Errorf("--link %q is neither a known editor (%s) nor a template containing {file}",
					o.link, strings.Join(html.Editors(), ", "))
			}
			template = o.link
		}
		opts = append(opts, html.SourceLink(template))
	}

	return html.New(opts...), nil
}

// The values each choice flag takes, written down once. The parser below, the usage
// line and the shell completion all read these, and a test checks that everything
// offered is something a parser accepts.
var (
	rankDirs   = []string{"LR", "TB"}
	dotThemes  = []string{"light", "dark"}
	portModes  = []string{"auto", "on", "off"}
	htmlThemes = []string{"auto", "light", "dark"}
	layouts    = []string{"graphviz", "dagre"}
)

// orList reads them out as a sentence would: "LR or TB", "auto, on or off".
func orList(values []string) string {
	if len(values) < 2 {
		return strings.Join(values, "")
	}
	return strings.Join(values[:len(values)-1], ", ") + " or " + values[len(values)-1]
}

func (o *dotOptions) parseRankDir() (dot.RankDirection, error) {
	switch strings.ToUpper(o.rankDir) {
	case "LR":
		return dot.LR, nil
	case "TB":
		return dot.TB, nil
	}
	return "", fmt.Errorf("--rankdir %q: want %s", o.rankDir, orList(rankDirs))
}

func (o *dotOptions) parseTheme() (dot.ThemeName, error) {
	switch strings.ToLower(o.theme) {
	case "light":
		return dot.Light, nil
	case "dark":
		return dot.Dark, nil
	}
	return "", fmt.Errorf("--theme %q: want %s", o.theme, orList(dotThemes))
}

func (o *dotOptions) parsePorts() (dot.PortMode, error) {
	switch strings.ToLower(o.ports) {
	case "auto":
		return dot.PortsAuto, nil
	case "on":
		return dot.PortsOn, nil
	case "off":
		return dot.PortsOff, nil
	}
	return 0, fmt.Errorf("--ports %q: want %s", o.ports, orList(portModes))
}

func (o *htmlOptions) parseTheme() (html.ThemeName, error) {
	switch strings.ToLower(o.theme) {
	case "auto":
		return html.Auto, nil
	case "light":
		return html.Light, nil
	case "dark":
		return html.Dark, nil
	}
	return "", fmt.Errorf("--theme %q: want %s", o.theme, orList(htmlThemes))
}

func (o *htmlOptions) parseLayout() (html.LayoutEngine, error) {
	switch strings.ToLower(o.layout) {
	case "graphviz":
		return html.Graphviz, nil
	case "dagre":
		return html.Dagre, nil
	}
	return "", fmt.Errorf("--layout %q: want %s", o.layout, orList(layouts))
}
