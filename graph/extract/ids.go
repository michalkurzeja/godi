package extract

import (
	"fmt"
	"strings"

	"github.com/michalkurzeja/godi/v2/di"
	"github.com/michalkurzeja/godi/v2/graph"
	"github.com/michalkurzeja/godi/v2/internal/util"
)

// assignIDs walks the scope tree top down, naming every scope and definition.
//
// The container only keeps parent pointers, so the tree is rebuilt here. A child
// scope's own name is a uuid, so it is named after its owner's node ID instead.
func (x *extractor) assignIDs() {
	owners := x.childScopeOwners()

	x.assignScope(x.container.Root(), graph.ScopeID(di.RootScope), "", 0, owners)

	// Scopes created directly through Scope.NewChild belong to no definition, so
	// the walk above never reaches them. Attach them to their parent by raw name.
	for scope := range x.container.Scopes() {
		if _, ok := x.scopeIDs[scope]; ok {
			continue
		}
		parent := graph.ScopeID(di.RootScope)
		depth := 1
		if p := scope.Parent(); p != nil {
			if id, ok := x.scopeIDs[p]; ok {
				parent = id
				depth = x.depthOf(id) + 1
			}
		}
		x.assignScope(scope, parent+graph.ScopeID("/scope:"+scope.Name()), parent, depth, owners)
		// Worth saying, because such a scope is drawn without an owner and
		// nothing in the container explains it. It is not a fault: a compiler
		// pass is entitled to make one.
		x.note(graph.SeverityInfo, graph.Diagnostic{
			Scope:   x.scopeIDs[scope],
			Message: fmt.Sprintf("scope %q belongs to no definition", scope.Name()),
		})
	}
}

// childScopeOwners maps every child scope to the definition that declared it.
func (x *extractor) childScopeOwners() map[*di.Scope]any {
	owners := make(map[*di.Scope]any)
	for _, def := range x.container.ServiceDefinitionsSeq() {
		if cs := def.ChildScope(); cs != nil {
			owners[cs] = def
		}
	}
	for _, def := range x.container.FunctionDefinitionsSeq() {
		if cs := def.ChildScope(); cs != nil {
			owners[cs] = def
		}
	}
	return owners
}

func (x *extractor) assignScope(scope *di.Scope, id, parent graph.ScopeID, depth int, owners map[*di.Scope]any) {
	if scope == nil {
		return
	}
	if _, done := x.scopeIDs[scope]; done {
		return
	}
	x.scopeIDs[scope] = id

	entry := &graph.Scope{ID: id, Parent: parent, Depth: depth, Name: scope.Name()}
	if owner, ok := owners[scope]; ok {
		entry.Owner = x.ownerID(owner)
		entry.OwnerName = x.ownerName(owner)
	}
	x.out.Scopes = append(x.out.Scopes, entry)

	// Name the definitions before recursing. A child scope is named after the
	// node that declared it.
	for def := range scope.ServiceDefinitionsSeq() {
		nodeID := x.mint(id, "svc", util.Signature(def.Type()), def.Labels())
		x.svcIDs[def] = nodeID
		x.byUUID[def.ID()] = nodeID
	}
	for def := range scope.FunctionDefinitionsSeq() {
		nodeID := x.mint(id, "fn", def.Func().Name(), def.Labels())
		x.funIDs[def] = nodeID
		x.byUUID[def.ID()] = nodeID
	}

	for def := range scope.ServiceDefinitionsSeq() {
		if cs := def.ChildScope(); cs != nil && owners[cs] == def {
			x.assignScope(cs, graph.ScopeID(x.svcIDs[def]), id, depth+1, owners)
		}
	}
	for def := range scope.FunctionDefinitionsSeq() {
		if cs := def.ChildScope(); cs != nil && owners[cs] == def {
			x.assignScope(cs, graph.ScopeID(x.funIDs[def]), id, depth+1, owners)
		}
	}
}

func (x *extractor) ownerID(owner any) graph.NodeID {
	switch def := owner.(type) {
	case *di.ServiceDefinition:
		return x.svcIDs[def]
	case *di.FunctionDefinition:
		return x.funIDs[def]
	}
	return ""
}

// ownerName is what to call the definition that declared a scope. It is what a
// node of it is called: a service by the type it provides, a function by its name.
//
// It is recorded on the scope for two reasons. A node ID is built to be unique
// rather than readable, and the owner may be filtered out of a graph that still
// has to name the scope it left behind.
func (x *extractor) ownerName(owner any) string {
	switch def := owner.(type) {
	case *di.ServiceDefinition:
		return util.Signature(def.Type())
	case *di.FunctionDefinition:
		return def.Func().Name()
	}
	return ""
}

func (x *extractor) depthOf(id graph.ScopeID) int {
	for _, s := range x.out.Scopes {
		if s.ID == id {
			return s.Depth
		}
	}
	return 0
}

// mint builds a readable, build-stable node ID. Definition uuids are new on every
// build, so an ID made from one could not be diffed or compared.
func (x *extractor) mint(scope graph.ScopeID, kind, name string, labels []di.Label) graph.NodeID {
	var sb strings.Builder
	sb.WriteString(string(scope))
	sb.WriteString("/")
	sb.WriteString(kind)
	sb.WriteString(":")
	sb.WriteString(name)

	if len(labels) > 0 {
		sb.WriteString("[")
		sb.WriteString(strings.Join(x.labelStrings(labels), ","))
		sb.WriteString("]")
	}

	base := sb.String()
	n := x.minted[base]
	x.minted[base] = n + 1
	if n > 0 {
		return graph.NodeID(fmt.Sprintf("%s#%d", base, n))
	}
	return graph.NodeID(base)
}
