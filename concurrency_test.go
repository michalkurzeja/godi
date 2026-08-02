package di_test

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	di "github.com/michalkurzeja/godi/v2"
)

// resolveFrom asks for a service from n goroutines at once and returns what they
// were given.
//
// The goroutines are all parked at a gate before any of them resolves. Spawning
// them and letting each run when it is scheduled is not the same thing: the
// first can finish before the last exists, and the test then passes without any
// two calls ever meeting.
//
// It fails on a deadline rather than blocking. A deadlock inside the container
// does not panic unless every goroutine in the process is asleep, so without
// this a broken lock would stall the whole package until the binary times out.
func resolveFrom[T any](t *testing.T, c di.Container, n int) []T {
	t.Helper()

	type result struct {
		svc T
		err error
	}

	var (
		results = make(chan result, n)
		ready   = make(chan struct{}, n)
		start   = make(chan struct{})
	)

	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			ready <- struct{}{}
			<-start

			svc, err := di.SvcByType[T](c)
			results <- result{svc: svc, err: err}
		})
	}

	for range n {
		<-ready // Everyone is at the gate; releasing it now means something.
	}
	close(start)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out resolving: the container is deadlocked")
	}

	close(results)

	svcs := make([]T, 0, n)
	for res := range results {
		require.NoError(t, res.err)
		svcs = append(svcs, res.svc)
	}
	return svcs
}

func TestALazyServiceIsBuiltOnceHoweverManyGoroutinesAskForIt(t *testing.T) {
	t.Parallel()

	var built atomic.Int64

	c, err := di.New().
		Services(
			di.Svc(func() *Logger {
				built.Add(1)
				time.Sleep(time.Millisecond) // Widen the window the goroutines used to race through.
				return NewLogger()
			}),
		).
		Build()
	require.NoError(t, err)

	got := resolveFrom[*Logger](t, c, 16)

	require.EqualValues(t, 1, built.Load(), "the factory ran more than once")
	for _, svc := range got {
		require.Same(t, got[0], svc, "goroutines were handed different instances")
	}
}

// Gate is configured by a method call the test can hold open, so a second
// goroutine can try to read the service while it is built but not yet
// configured.
type Gate struct {
	configuring chan struct{} // Closed when the method call starts.
	release     chan struct{} // Closed by the test to let the method call finish.
	name        string
}

func (g *Gate) SetName(name string) {
	close(g.configuring)
	<-g.release
	g.name = name
}

func TestAServiceIsNotVisibleUntilItsMethodCallsHaveRun(t *testing.T) {
	t.Parallel()

	gate := &Gate{configuring: make(chan struct{}), release: make(chan struct{})}

	c, err := di.New().
		Services(
			di.Svc(func() *Gate { return gate }).MethodCall((*Gate).SetName, "configured"),
		).
		Build()
	require.NoError(t, err)

	go func() { _, _ = di.SvcByType[*Gate](c) }()

	<-gate.configuring // Built, and now stuck part way through being configured.

	second := make(chan *Gate, 1)
	go func() {
		svc, _ := di.SvcByType[*Gate](c)
		second <- svc
	}()

	select {
	case svc := <-second:
		t.Fatalf("a second goroutine was handed the service mid-configuration (name=%q)", svc.name)
	case <-time.After(100 * time.Millisecond):
	}

	close(gate.release)

	select {
	case svc := <-second:
		require.Equal(t, "configured", svc.name)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out: the container is deadlocked")
	}
}

func TestAFactoryCannotResolveFromTheContainerBuildingIt(t *testing.T) {
	t.Parallel()

	var (
		c        di.Container
		innerErr error
	)

	c, err := di.New().
		Services(
			di.Svc(func() *Logger {
				_, innerErr = di.SvcByType[*Banner](c)
				return NewLogger()
			}),
			di.Svc(NewBanner),
		).
		Build()
	require.NoError(t, err)

	_, err = di.SvcByType[*Logger](c)
	require.NoError(t, err)

	require.ErrorContains(t, innerErr, "must not resolve from the container that is building it")
}

func TestAFactoryThatFailedIsTriedAgainByTheNextCall(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int64

	c, err := di.New().
		Services(
			di.Svc(func() (*Logger, error) {
				if attempts.Add(1) == 1 {
					return nil, errors.New("first attempt fails")
				}
				return NewLogger(), nil
			}),
		).
		Build()
	require.NoError(t, err)

	_, err = di.SvcByType[*Logger](c)
	require.ErrorContains(t, err, "first attempt fails")

	require.NotNil(t, resolveFrom[*Logger](t, c, 1)[0])
	require.EqualValues(t, 2, attempts.Load())
}

// A panicking factory must give the build lock back, or every later call blocks
// on it forever.
func TestAPanickingFactoryLeavesTheContainerUsable(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int64

	c, err := di.New().
		Services(
			di.Svc(func() *Logger {
				if attempts.Add(1) == 1 {
					panic("first attempt panics")
				}
				return NewLogger()
			}),
		).
		Build()
	require.NoError(t, err)

	require.Panics(t, func() { _, _ = di.SvcByType[*Logger](c) })

	require.NotNil(t, resolveFrom[*Logger](t, c, 1)[0])
}
