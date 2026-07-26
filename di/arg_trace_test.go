package di_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	godi "github.com/michalkurzeja/godi/v2"
	"github.com/michalkurzeja/godi/v2/di"
)

type traceStore struct{}

func newTraceStore() *traceStore { return &traceStore{} }

type traceReporter interface{ report() }

type traceConsole struct{}

func (traceConsole) report() {}

func newTraceConsole() traceConsole { return traceConsole{} }

type traceSvc struct{}

func newTraceSvc(traceReporter, *traceStore, string) *traceSvc { return &traceSvc{} }

type traceCollector struct{}

func newTraceCollector(...traceReporter) *traceCollector { return &traceCollector{} }

type traceLabelled struct{}

func newTraceLabelled(*traceStore) *traceLabelled { return &traceLabelled{} }

type traceByLabel struct{}

func newTraceByLabel(*traceLabelled) *traceByLabel { return &traceByLabel{} }

// Resolving an argument and tracing it are two walks over the same wiring, and
// nothing outside would notice them disagreeing: the container would keep
// working and only the graph would lie. So they are checked against each other.
func TestATraceMatchesWhatTheArgumentResolvesTo(t *testing.T) {
	t.Parallel()

	c, err := godi.New().
		Services(
			godi.Svc(newTraceStore),
			godi.Svc(newTraceConsole),
			godi.Svc(newTraceSvc, "literal"),
			godi.Svc(newTraceCollector),
			godi.Svc(newTraceLabelled).Labels("reports"),
			godi.Svc(newTraceByLabel, godi.Type[*traceLabelled]("reports")),
		).
		Build()
	require.NoError(t, err)

	container, ok := c.(*di.Container)
	require.True(t, ok)

	var checked int
	for _, def := range container.ServiceDefinitionsSeq() {
		for _, slot := range def.Factory().Args().Slots() {
			if !slot.IsFilled() {
				continue
			}
			checked++

			resolveScope := def.EffectiveScope()
			arg := slot.Arg()
			require.Equal(t,
				di.ResolveArgIDs(resolveScope, arg),
				matchesOf(di.TraceArg(resolveScope, arg)),
				"argument %d of %s: the trace and the resolver disagree", slot.Index(), def)
		}
	}
	require.NotZero(t, checked, "the fixture wired nothing worth checking")
}

// matchesOf flattens a trace into the definitions it resolved to, in order.
func matchesOf(t di.ArgTrace) []di.ID {
	var ids []di.ID
	ids = append(ids, t.Matches...)
	for _, part := range t.Parts {
		ids = append(ids, matchesOf(part)...)
	}
	return ids
}
