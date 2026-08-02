package extract

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michalkurzeja/godi/v2/di"
	"github.com/michalkurzeja/godi/v2/graph"
)

// A label part matching nothing inside a compound that resolved through a
// sibling part is not a fault - only when the whole argument matched nothing
// is it one. walk decides this from a match flag computed once over the whole
// trace tree, not from EdgeCount as it stands partway through, so the order a
// compound's parts are declared in must not change the answer.
func TestALabelFaultInsideAResolvedCompoundIsOrderIndependent(t *testing.T) {
	t.Parallel()

	const depID = di.ID("dep-1")

	labelPart := di.ArgTrace{
		Kind:  di.ArgKindLabel,
		Label: "nope",
		Fault: di.ArgFault{Kind: di.ArgFaultNoServicesWithLabel, Label: "nope"},
	}
	refPart := di.ArgTrace{Kind: di.ArgKindRef, Matches: []di.ID{depID}, By: di.ResolutionRef}

	tests := []struct {
		name  string
		parts []di.ArgTrace
	}{
		{"the label part first", []di.ArgTrace{labelPart, refPart}},
		{"the resolving part first", []di.ArgTrace{refPart, labelPart}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			x := newExtractor(nil, graph.NewConfig(), nil)
			x.byUUID[depID] = "root/svc:app.(*Dep)"

			trace := di.ArgTrace{Kind: di.ArgKindCompound, Parts: test.parts}
			p := &graph.Param{ID: "root/svc:app.(*User)#f:0"}

			x.walk(p, trace, hasMatch(trace))

			require.False(t, p.Faulty(), "a label matching nothing beside a part that resolved should not fault: %+v", p.Diagnostics)
		})
	}
}
