package di

import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"sort"
	"sync"

	"github.com/dominikbraun/graph"
	"github.com/dominikbraun/graph/draw"
	"github.com/hashicorp/go-multierror"
	"github.com/samber/lo"
)

// Node represents an entry in the dependency graph.
type Node interface {
	ID() string
	Type() reflect.Type
	Value(c Container) (any, error)
}

type nodeWithDependencies interface {
	providerDependencies() []Node
	deferredDependencies() []Node
}

type container struct {
	mx       sync.RWMutex
	graph    graph.Graph[string, *vertex]
	vertices []*vertex

	compiled bool
}

func New() Container {
	return &container{graph: graph.New(vertexHash, graph.Directed())}
}

func (c *container) Register(node Node) error {
	c.mx.Lock()
	defer c.mx.Unlock()

	if c.compiled {
		return ErrContainerCompiled
	}

	err := c.addNodeOrSwapRef(node)
	if errors.Is(err, graph.ErrVertexAlreadyExists) {
		return NodeAlreadyExistsError{ID: node.ID()}
	}
	if err != nil {
		return err
	}

	depNode, ok := node.(nodeWithDependencies)
	if !ok {
		return nil
	}

	// Add dependencies. These are injected on value creation, so they must be acyclic.
	for _, dep := range depNode.providerDependencies() {
		err := c.addNodeNX(dep)
		if err != nil {
			return err
		}

		cycle, err := graph.CreatesCycle(c.graph, node.ID(), dep.ID())
		if err != nil {
			return err
		}
		if cycle {
			return CyclicDependencyError{Nodes: [2]string{node.ID(), dep.ID()}}
		}

		err = c.addEdgeNX(node.ID(), dep.ID())
		if err != nil {
			return err
		}
	}

	// Add deferredTo dependencies. These are injected after value creation, so they can form cycles.
	for _, dep := range depNode.deferredDependencies() {
		err := c.addNodeNX(dep)
		if err != nil {
			return err
		}

		err = c.addEdgeNX(node.ID(), dep.ID())
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *container) addNodeOrSwapRef(node Node) error {
	v, err := c.graph.Vertex(node.ID())
	if errors.Is(err, graph.ErrVertexNotFound) {
		return c.addVertex(newVertex(node))
	}
	if err != nil {
		return err
	}

	if !v.IsRef() {
		return graph.ErrVertexAlreadyExists
	}

	v.SwapNode(node)
	return nil
}

func (c *container) addNodeNX(node Node) error {
	err := c.addVertex(newVertex(node))
	if errors.Is(err, graph.ErrVertexAlreadyExists) {
		return nil
	}
	if err != nil {
		return err
	}
	return nil
}

func (c *container) addVertex(v *vertex) error {
	err := c.graph.AddVertex(v)
	if err != nil {
		return err
	}
	c.vertices = append(c.vertices, v)
	return nil
}

func (c *container) addEdgeNX(from, to string) error {
	err := c.graph.AddEdge(from, to)
	if errors.Is(err, graph.ErrEdgeAlreadyExists) {
		return nil
	}
	if err != nil {
		return err
	}
	return nil
}

func (c *container) Get(id string) (Node, error) {
	c.mx.RLock()
	defer c.mx.RUnlock()

	if !c.compiled {
		return nil, ErrContainerNotCompiled
	}

	v, err := c.graph.Vertex(id)
	if errors.Is(err, graph.ErrVertexNotFound) {
		return nil, NodeNotFoundError{ID: id}
	}
	if err != nil {
		return nil, err
	}

	return v.node, nil
}

func (c *container) getImplementingVertex(ifaceNode Node) (*vertex, error) {
	iface := ifaceNode.Type()
	if iface.Kind() != reflect.Interface {
		return nil, fmt.Errorf("cannot find implementation: %s must be an interface", iface)
	}

	var impls []*vertex
	for _, v := range c.vertices {
		if v.node != ifaceNode && v.node.Type().Implements(iface) {
			impls = append(impls, v)
		}
	}

	if len(impls) == 0 {
		return nil, NoImplementationError{Typ: iface}
	}
	if len(impls) > 1 {
		implNames := lo.Map(impls, func(v *vertex, _ int) string { return v.node.ID() })
		sort.Strings(implNames)
		return nil, AmbiguousImplementationError{Typ: iface, Impls: implNames}
	}

	return impls[0], nil
}

func (c *container) Compile() error {
	c.mx.Lock()
	defer c.mx.Unlock()

	if c.compiled {
		return ErrContainerCompiled
	}

	m, err := c.graph.PredecessorMap()
	if err != nil {
		return err
	}

	var refErrs error
	for id, predecessors := range m {
		v, err := c.graph.Vertex(id)
		if err != nil {
			return err
		}

		if !v.IsRef() {
			continue
		}

		if v.node.Type().Kind() == reflect.Interface {
			implV, err := c.getImplementingVertex(v.node)
			if err != nil {
				refErrs = multierror.Append(refErrs, err)
				continue
			}

			v.SwapNode(newProxy(v.node.ID(), implV))
			err = c.graph.AddEdge(v.node.ID(), implV.node.ID())
			if err != nil {
				refErrs = multierror.Append(refErrs, err)
				continue
			}

			continue
		}

		refErrs = multierror.Append(refErrs, UnresolvedRefError{ID: id, RefNodes: lo.Keys(predecessors)})
	}
	if refErrs != nil {
		return refErrs
	}

	c.compiled = true

	return nil
}

// Export returns a DOT representation of the dependency graph.
func (c *container) Export(w io.Writer) error {
	c.mx.RLock()
	defer c.mx.RUnlock()

	if !c.compiled {
		return ErrContainerNotCompiled
	}

	err := draw.DOT(c.graph, w)
	if err != nil {
		return fmt.Errorf("di: %w", err)
	}

	return nil
}

type vertex struct {
	node Node
}

func newVertex(node Node) *vertex {
	return &vertex{node: node}
}

func vertexHash(v *vertex) string {
	return v.node.ID()
}

func (v *vertex) IsRef() bool {
	_, ok := v.node.(*ref)
	return ok
}

func (v *vertex) SwapNode(node Node) {
	v.node = node
}

// Nodes:

// ref is a placeholder for a node that has not been registered yet.
type ref struct {
	id string
	t  reflect.Type
}

func newRef(id string, t reflect.Type) *ref {
	return &ref{id: id, t: t}
}

func (r *ref) ID() string {
	return r.id
}

func (r *ref) Type() reflect.Type {
	return r.t
}

func (r *ref) Value(_ Container) (any, error) {
	return nil, fmt.Errorf("cannot instantiate node %s: cannot instantiate a reference", r.id)
}

// proxy is a pass-thru node that proxies a dependency to another node.
// It is used to connect interfaces with their implementations.
type proxy struct {
	id     string
	target *vertex
}

func newProxy(id string, target *vertex) *proxy {
	return &proxy{id: id, target: target}
}

func (p *proxy) ID() string {
	return p.id
}

func (p *proxy) Type() reflect.Type {
	return p.target.node.Type()
}

func (p *proxy) Value(c Container) (any, error) {
	node, err := c.Get(p.target.node.ID())
	if err != nil {
		return nil, err
	}

	return node.Value(c)
}

func (p *proxy) providerDependencies() []Node {
	depNode, ok := p.target.node.(nodeWithDependencies)
	if !ok {
		return nil
	}
	return depNode.providerDependencies()
}

func (p *proxy) deferredDependencies() []Node {
	depNode, ok := p.target.node.(nodeWithDependencies)
	if !ok {
		return nil
	}
	return depNode.deferredDependencies()
}
