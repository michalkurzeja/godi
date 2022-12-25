package di

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

var (
	ErrContainerCompiled    = errors.New("di: container is already compiled")
	ErrContainerNotCompiled = errors.New("di: container must be compiled first")
)

type NodeAlreadyExistsError struct {
	ID string
}

func (err NodeAlreadyExistsError) Error() string {
	return fmt.Sprintf("node %s already exists", err.ID)
}

type NodeNotFoundError struct {
	ID string
}

func (err NodeNotFoundError) Error() string {
	return fmt.Sprintf("node %s not found", err.ID)
}

type NoImplementationError struct {
	Typ reflect.Type
}

func (err NoImplementationError) Error() string {
	return fmt.Sprintf("no implementation of %s found", fqn(err.Typ))
}

type AmbiguousImplementationError struct {
	Typ   reflect.Type
	Impls []string
}

func (err AmbiguousImplementationError) Error() string {
	return fmt.Sprintf("multiple implementations of %s found: %s", fqn(err.Typ), strings.Join(err.Impls, ", "))
}

type CyclicDependencyError struct {
	Nodes [2]string
}

func (err CyclicDependencyError) Error() string {
	return fmt.Sprintf("detected cyclic dependency between %s and %s", err.Nodes[0], err.Nodes[1])
}

type UnresolvedRefError struct {
	ID       string
	RefNodes []string
}

func (err UnresolvedRefError) Error() string {
	return fmt.Sprintf("node %s is not registered but is referenced by nodes %s", err.ID, err.RefNodes)
}
