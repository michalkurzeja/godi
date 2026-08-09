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
		severity := x.severityOf(d.Severity)
		x.record(x.destination(d.Site, severity, claimed), graph.Diagnostic{
			Severity: severity,
			Message:  d.Message,
			Pass:     d.Pass,
			Location: x.registered(d.At),
		})
		x.candidateEdges(d)
	}
}

// candidateEdges draws what a pass could not choose between, against the argument
// that could not choose.
//
// It is the second route to an edge, and the only one open to an argument that
// resolved to nothing: a pass that stops the build leaves every slot after it
// empty, and an empty slot has no argument to trace. What was on offer is then
// the pass's own account or nothing.
//
// An argument that already produced edges keeps them. A pass naming what the
// wiring found is saying it again, not adding to it.
func (x *extractor) candidateEdges(d di.Diagnostic) {
	p := x.slotParams[d.Site.Slot()]
	if p == nil || p.EdgeCount > 0 {
		return
	}

	for _, related := range d.Related {
		node := x.siteNode(related)
		if node == nil {
			continue
		}
		if edge := x.edge(p, node.ID, graph.ResolutionByType, nil); edge != nil {
			edge.Candidate = true
		}
	}
}

// destination is what a diagnostic goes on: the argument it named, else the
// definition, else the scope, else the graph. Each fallback is a step outwards to
// something that does exist, so a diagnostic loses the element it named but never
// the words.
func (x *extractor) destination(site di.Site, severity graph.Severity, claimed map[*graph.Param]bool) *[]graph.Diagnostic {
	// A site naming an argument godi injects itself lands nowhere: the receiver
	// slot of a method call produces no param of its own.
	if p := x.slotParams[site.Slot()]; p != nil {
		// The pass saw more than the trace could, and said it in the words the
		// build failed with — but only an error-severity diagnostic is a
		// correction of that fault. An info or warning naming the same slot is
		// additional context, not a replacement, so it must not erase the error
		// extraction already recorded there.
		if !claimed[p] {
			claimed[p] = true
			if severity == graph.SeverityError {
				p.Diagnostics = nil
			}
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
