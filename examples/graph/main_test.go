package main

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/michalkurzeja/godi/v2/graph"
)

// Both ways out of run() extract the graph, and they have to extract the same
// one. -open once did it without the options the stdout path used, so the fake
// DSN and the address a compiler pass substitutes were in the DOT and missing
// from the page. Two different graphs from one command, each looking fine.
func TestOpenDrawsTheSameGraphAsStdout(t *testing.T) {
	var opened *graph.Graph

	restore := openGraph
	openGraph = func(g *graph.Graph, _ graph.Encoder) (string, error) {
		opened = g
		return "", nil
	}
	t.Cleanup(func() { openGraph = restore })

	var out bytes.Buffer
	require(t, run("dot", "", "", "", false, false, &out))
	require(t, run("dot", "", "", "", true, false, io.Discard))

	if opened == nil {
		t.Fatal("the -open path never extracted a graph")
	}

	const literal = "127.0.0.1:9090"
	if !strings.Contains(out.String(), literal) {
		t.Fatalf("the stdout graph is missing the literal %q, so this test proves nothing", literal)
	}
	if !hasLiteral(opened, literal) {
		t.Errorf("the -open graph has no literal %q: the two paths draw different graphs", literal)
	}
}

func hasLiteral(g *graph.Graph, want string) bool {
	for _, node := range g.Nodes {
		for _, param := range node.Params {
			for _, lit := range param.Literals {
				if strings.Contains(lit.Value, want) {
					return true
				}
			}
		}
	}
	return false
}

func require(t *testing.T, err error) {
	t.Helper()

	if err != nil {
		t.Fatal(err)
	}
}

// The wiring as declared is a graph too, and the one to reach for when the
// container will not build. It has to say that is what it is.
func TestSnapshotDrawsTheWiringBeforeItIsCompiled(t *testing.T) {
	var out bytes.Buffer
	require(t, run("text", "", "", "", false, true, &out))

	if !strings.Contains(out.String(), "snapshot: taken during the graph snapshot pass") {
		t.Errorf("the snapshot does not say it is one:\n%s", out.String())
	}
	if strings.Contains(out.String(), "autowiring") {
		t.Error("nothing should be autowired before the container is compiled")
	}
}
