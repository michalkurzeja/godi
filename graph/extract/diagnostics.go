package extract

import (
	"github.com/michalkurzeja/godi/v2/di"
	"github.com/michalkurzeja/godi/v2/graph"
)

// reportedDiagnostics puts what the compiler passes objected to onto the elements
// they objected to.
//
// This is the only thing that tells the graph about a fault the arguments cannot
// describe themselves: a binding of the wrong shape, an ambiguous type, a factory
// that returned an error. Resolving an argument and tracing it are two walks over
// the same wiring, and only the first one validates.
func (x *extractor) reportedDiagnostics() {
	// The arguments a pass has already spoken about. What extraction guessed about
	// one goes the first time a pass names it, and stays gone if a second does.
	claimed := make(map[*graph.Param]bool)

	for _, d := range x.container.Diagnostics() {
		x.record(x.destination(d.Site, claimed), graph.Diagnostic{
			Severity: x.severityOf(d.Severity),
			Message:  d.Message,
			Pass:     d.Pass,
			Location: x.registered(d.At),
		})
	}
}

// destination is what a diagnostic goes on: the argument it named, else the
// definition, else the scope, else the graph. Each fallback is a step outwards to
// something that does exist, so a diagnostic loses the element it named but never
// the words.
func (x *extractor) destination(site di.Site, claimed map[*graph.Param]bool) *[]graph.Diagnostic {
	// A site naming an argument godi injects itself lands nowhere: the receiver
	// slot of a method call produces no param of its own.
	if p := x.slotParams[site.Slot()]; p != nil {
		// The pass saw more than the trace could, and said it in the words the
		// build failed with.
		if !claimed[p] {
			p.Diagnostics, claimed[p] = nil, true
		}
		return &p.Diagnostics
	}
	if node := x.siteNode(site); node != nil {
		return &node.Diagnostics
	}
	if scope := x.scopeEntries[site.Scope()]; scope != nil {
		return &scope.Diagnostics
	}
	// A definition that never made it into the container, a pass that could not be
	// scheduled: nothing narrower is left.
	return &x.out.Diagnostics
}

func (x *extractor) siteNode(site di.Site) *graph.Node {
	if def := site.Service(); def != nil {
		return x.nodeEntries[x.svcIDs[def]]
	}
	if def := site.Function(); def != nil {
		return x.nodeEntries[x.funIDs[def]]
	}
	return nil
}

func (x *extractor) severityOf(severity di.Severity) graph.Severity {
	switch severity {
	case di.SeverityInfo:
		return graph.SeverityInfo
	case di.SeverityWarning:
		return graph.SeverityWarning
	case di.SeverityError:
		return graph.SeverityError
	}
	return graph.SeverityWarning
}
