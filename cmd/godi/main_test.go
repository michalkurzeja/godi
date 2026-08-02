package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/michalkurzeja/godi/v2/graph"
	"github.com/michalkurzeja/godi/v2/graph/html"
)

// The CLI is the only way a rendered graph reaches anyone: the library writes
// JSON and stops there. What is pinned here is that every format is reachable,
// that the flags actually arrive at the encoder, and that a graph from a godi
// the CLI does not recognise is still shown.

const fixture = "testdata/graph.json"

type result struct {
	stdout string
	stderr string
	err    error
}

func run(t *testing.T, args ...string) result {
	t.Helper()
	return runWithStdin(t, nil, args...)
}

func runWithStdin(t *testing.T, stdin io.Reader, args ...string) result {
	t.Helper()

	var stdout, stderr bytes.Buffer

	cmd := newRootCmd(&stdout, &stderr)
	cmd.SetArgs(args)
	if stdin != nil {
		cmd.SetIn(stdin)
	}

	err := cmd.Execute()

	return result{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

// A script that forgets the format subcommand should see a non-zero exit and
// nothing on stdout, not a page of help text standing in for a graph.
func TestExportWithNoFormatIsAnError(t *testing.T) {
	t.Parallel()

	got := run(t, "export")

	require.Error(t, got.err)
	require.ErrorContains(t, got.err, "a format is required")
	require.Empty(t, got.stdout, "help text must not stand in for the graph a script expected")
}

func TestExportText(t *testing.T) {
	t.Parallel()

	got := run(t, "export", "text", fixture)
	require.NoError(t, got.err)

	require.Contains(t, got.stdout, "main.(*Server)")
	require.Contains(t, got.stdout, "registered:", "locations are on by default")
}

func TestExportTextTakesItsFlags(t *testing.T) {
	t.Parallel()

	got := run(t, "export", "text", "--no-locations", fixture)
	require.NoError(t, got.err)

	require.Contains(t, got.stdout, "main.(*Server)")
	require.NotContains(t, got.stdout, "registered:")
}

func TestExportDot(t *testing.T) {
	t.Parallel()

	got := run(t, "export", "dot", fixture)
	require.NoError(t, got.err)

	require.Contains(t, got.stdout, "digraph godi {")
	require.Contains(t, got.stdout, "rankdir=LR", "the default reaches the encoder")
}

func TestExportDotTakesItsFlags(t *testing.T) {
	t.Parallel()

	got := run(t, "export", "dot", "--rankdir", "TB", "--theme", "dark", fixture)
	require.NoError(t, got.err)

	require.Contains(t, got.stdout, "rankdir=TB")
	require.Contains(t, got.stdout, "#0d1117", "the dark palette's background")
}

func TestExportHTML(t *testing.T) {
	t.Parallel()

	got := run(t, "export", "html", "--title", "my wiring", "--layout", "dagre", fixture)
	require.NoError(t, got.err)

	require.Contains(t, got.stdout, "<!DOCTYPE html>")
	require.Contains(t, got.stdout, "my wiring")
	require.NotContains(t, got.stdout, "viz-global", "only the chosen engine is embedded")
}

// A round trip through the CLI has to produce something the CLI can read again,
// or the format is not an interchange format.
func TestExportJSONRoundTrips(t *testing.T) {
	t.Parallel()

	got := run(t, "export", "json", "--indent", "  ", fixture)
	require.NoError(t, got.err)
	require.Contains(t, got.stdout, "\n  \"metadata\": {")

	g, md, err := graph.ReadJSON(strings.NewReader(got.stdout))
	require.NoError(t, err)
	require.Equal(t, graph.Schema, md.Schema)
	require.NotEmpty(t, g.Nodes)
}

// Reading a pipe is what makes the export commands worth chaining.
func TestAGraphCanArriveOnStandardInput(t *testing.T) {
	t.Parallel()

	f, err := os.Open(fixture)
	require.NoError(t, err)
	defer f.Close()

	got := runWithStdin(t, f, "export", "text")
	require.NoError(t, got.err)
	require.Contains(t, got.stdout, "main.(*Server)")
}

func TestOutputCanGoToAFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "graph.dot")

	got := run(t, "export", "dot", "-o", path, fixture)
	require.NoError(t, got.err)
	require.Empty(t, got.stdout, "nothing goes to standard output when a file was named")

	written, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(written), "digraph godi {")
}

// A link template is either a known editor or one of your own. Anything else is
// a typo, and rendering a page whose links go nowhere would hide it.
func TestTheEditorLinkIsAPresetOrATemplate(t *testing.T) {
	t.Parallel()

	preset := run(t, "export", "html", "--link", "vscode", fixture)
	require.NoError(t, preset.err)
	require.Contains(t, preset.stdout, "vscode://file")

	own := run(t, "export", "html", "--link", "myeditor://open?at={file}#{line}", fixture)
	require.NoError(t, own.err)
	require.Contains(t, own.stdout, "myeditor://open")

	typo := run(t, "export", "html", "--link", "vscoed", fixture)
	require.ErrorContains(t, typo.err, `--link "vscoed" is neither a known editor`)
	require.Contains(t, typo.err.Error(), "vscode", "the message lists what it would accept")
}

func TestBadFlagValuesAreReportedByName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{"rankdir", []string{"export", "dot", "--rankdir", "sideways"}, `--rankdir "sideways": want LR or TB`},
		{"dot theme", []string{"export", "dot", "--theme", "puce"}, `--theme "puce": want light or dark`},
		{"ports", []string{"export", "dot", "--ports", "maybe"}, `--ports "maybe": want auto, on or off`},
		{"html theme", []string{"export", "html", "--theme", "puce"}, `--theme "puce": want auto, light or dark`},
		{"layout", []string{"export", "html", "--layout", "neato"}, `--layout "neato": want graphviz or dagre`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			require.ErrorContains(t, run(t, append(test.args, fixture)...).err, test.want)
		})
	}
}

func TestAFileThatIsNotAGraphIsReported(t *testing.T) {
	t.Parallel()

	require.ErrorContains(t, run(t, "export", "text", "testdata").err, "is a directory")
	require.ErrorContains(t, run(t, "export", "text", "nope.json").err, "no such file or directory")

	require.ErrorContains(t,
		runWithStdin(t, strings.NewReader("not json"), "export", "text").err,
		"graph: reading json")
}

// The CLI and the library are installed separately and will drift. Refusing to
// draw a graph because its schema is a version off would leave the reader with
// nothing at the moment they need it most.
func TestAGraphFromAnotherGodiIsStillDrawn(t *testing.T) {
	t.Parallel()

	original, err := os.ReadFile(fixture)
	require.NoError(t, err)

	var doc map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(original, &doc))
	doc["metadata"] = json.RawMessage(`{"schema":"godi.graph/v99"}`)

	edited, err := json.Marshal(doc)
	require.NoError(t, err)

	got := runWithStdin(t, bytes.NewReader(edited), "export", "text")
	require.NoError(t, got.err)

	require.Contains(t, got.stdout, "main.(*Server)", "the graph is drawn anyway")
	require.Contains(t, got.stdout, "godi.graph/v99", "and it says which godi wrote it")
}

// complete drives cobra's completion machinery the way a shell does, and returns
// what the shell would offer along with the directive that says how to treat it.
func complete(t *testing.T, args ...string) (values []string, directive string) {
	t.Helper()

	got := run(t, append([]string{cobra.ShellCompRequestCmd}, args...)...)
	require.NoError(t, got.err)

	for line := range strings.SplitSeq(strings.TrimSpace(got.stdout), "\n") {
		if after, found := strings.CutPrefix(line, ":"); found {
			return values, after
		}
		value, _, _ := strings.Cut(line, "\t") // Anything after a tab is a description.
		values = append(values, value)
	}

	t.Fatalf("no completion directive in %q", got.stdout)
	return nil, ""
}

// Without this, the shell offers filenames where a choice belongs. That is useless,
// and it misleads about what the flag takes.
func TestChoiceFlagsCompleteToTheirValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{"dot rankdir", []string{"export", "dot", "--rankdir", ""}, rankDirs},
		{"dot theme", []string{"export", "dot", "--theme", ""}, dotThemes},
		{"dot ports", []string{"export", "dot", "--ports", ""}, portModes},
		{"html theme", []string{"export", "html", "--theme", ""}, htmlThemes},
		{"html layout", []string{"export", "html", "--layout", ""}, layouts},
		{"view layout", []string{"view", "--layout", ""}, layouts},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			values, directive := complete(t, test.args...)
			require.Equal(t, test.want, values)
			require.Equal(t, "4", directive, "ShellCompDirectiveNoFileComp: a choice is not a filename")
		})
	}
}

func TestTheEditorLinkCompletesToTheEditorsItKnows(t *testing.T) {
	t.Parallel()

	values, _ := complete(t, "export", "html", "--link", "")
	require.Equal(t, html.Editors(), values)
}

func TestTheGraphArgumentCompletesToJSONFiles(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{
		{"export", "text", ""},
		{"export", "dot", ""},
		{"export", "html", ""},
		{"export", "json", ""},
		{"view", ""},
	} {
		values, directive := complete(t, args...)
		require.Equal(t, []string{"json"}, values, "%v", args)
		require.Equal(t, "8", directive, "ShellCompDirectiveFilterFileExt")
	}

	// One file each, so there is nothing to offer for a second.
	_, directive := complete(t, "export", "text", fixture, "")
	require.Equal(t, "4", directive)
}

// The values a flag offers and the values it accepts are written in two places.
// A name in one but not the other is a flag that tab-completes to an error.
func TestEveryOfferedValueIsAccepted(t *testing.T) {
	t.Parallel()

	tests := []struct {
		flag   string
		values []string
		parse  func(string) error
	}{
		{"--rankdir", rankDirs, func(s string) error { _, err := (&dotOptions{rankDir: s}).parseRankDir(); return err }},
		{"--theme (dot)", dotThemes, func(s string) error { _, err := (&dotOptions{theme: s}).parseTheme(); return err }},
		{"--ports", portModes, func(s string) error { _, err := (&dotOptions{ports: s}).parsePorts(); return err }},
		{"--theme (html)", htmlThemes, func(s string) error { _, err := (&htmlOptions{theme: s}).parseTheme(); return err }},
		{"--layout", layouts, func(s string) error { _, err := (&htmlOptions{layout: s}).parseLayout(); return err }},
	}

	for _, test := range tests {
		t.Run(test.flag, func(t *testing.T) {
			t.Parallel()

			require.NotEmpty(t, test.values)
			for _, value := range test.values {
				require.NoError(t, test.parse(value), "%s offers %q", test.flag, value)
			}
		})
	}

	for _, editor := range html.Editors() {
		_, ok := html.EditorLink(editor)
		require.True(t, ok, "--link offers %q", editor)
	}
}

func TestOrListReadsAsASentence(t *testing.T) {
	t.Parallel()

	require.Empty(t, orList(nil))
	require.Equal(t, "LR", orList([]string{"LR"}))
	require.Equal(t, "LR or TB", orList([]string{"LR", "TB"}))
	require.Equal(t, "auto, on or off", orList([]string{"auto", "on", "off"}))
}

// The port is usually chosen for us, so it has to be on screen before the
// browser is pointed anywhere.
func TestViewServesUntilItIsStopped(t *testing.T) {
	t.Parallel()

	var stdout, stderr syncBuffer

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	cmd := newRootCmd(&stdout, &stderr)
	cmd.SetArgs([]string{"view", "--no-open", "--addr", "127.0.0.1:0", fixture})

	done := make(chan error, 1)
	go func() { done <- cmd.ExecuteContext(ctx) }()

	url := awaitURL(t, &stderr)

	res, err := http.Get(url) //nolint:noctx // Our own loopback server, in a test.
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, res.StatusCode)
	require.Equal(t, "text/html; charset=utf-8", res.Header.Get("Content-Type"))

	// Before cancelling: Shutdown waits for connections in flight, and a body
	// left open is one.
	_, err = io.Copy(io.Discard, res.Body)
	require.NoError(t, err)
	require.NoError(t, res.Body.Close())

	cancel()
	require.NoError(t, <-done, "a server told to stop has not failed")
}

// The page keeps the reader's settings against the origin it was served from, so
// a port chosen fresh each run loses them every time. The default is fixed for
// that reason, and gives way rather than failing when something already has it.
func TestViewFallsBackWhenItsUsualPortIsTaken(t *testing.T) {
	t.Parallel()

	// Whatever the default is, hold it, so the run below has to give way. Bound
	// on this machine only, and released when the test ends.
	held, err := net.Listen("tcp", defaultAddr)
	if err != nil {
		t.Skipf("cannot hold %s to test the fallback: %s", defaultAddr, err)
	}
	defer held.Close()

	var stdout, stderr syncBuffer

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	cmd := newRootCmd(&stdout, &stderr)
	cmd.SetArgs([]string{"view", "--no-open", fixture}) // No --addr: the default is the point.

	done := make(chan error, 1)
	go func() { done <- cmd.ExecuteContext(ctx) }()

	url := awaitURL(t, &stderr)
	require.NotContains(t, url, defaultAddr, "the held port was taken anyway")
	require.Contains(t, stderr.String(), "is taken, serving on a free port instead",
		"a reader whose settings will not be there has to be told why")

	res, err := http.Get(url) //nolint:noctx // Our own loopback server, in a test.
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, res.StatusCode)
	require.NoError(t, res.Body.Close())

	cancel()
	require.NoError(t, <-done)
}

// An address asked for by name is one the caller means, so it fails rather than
// quietly serving somewhere else.
func TestViewFailsWhenTheAddressItWasGivenIsTaken(t *testing.T) {
	t.Parallel()

	held, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer held.Close()

	res := run(t, "view", "--no-open", "--addr", held.Addr().String(), fixture)
	require.ErrorContains(t, res.err, "serve: listening on")
}

// syncBuffer is a buffer the test can read while the command writes to it from
// its own goroutine.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.String()
}

// awaitURL waits for view to report where it bound. The buffer is written from
// the command's goroutine, so this polls rather than reading once.
func awaitURL(t *testing.T, stderr *syncBuffer) string {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		_, after, found := strings.Cut(stderr.String(), "godi: serving on ")
		if found {
			url, _, ok := strings.Cut(after, "\n")
			if ok {
				return url
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("view never said where it was serving; stderr was %q", stderr.String())
	return ""
}
