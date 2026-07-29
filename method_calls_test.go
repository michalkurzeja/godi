package di_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	di "github.com/michalkurzeja/godi/v2"
)

// Logger is configured by a method call rather than by its factory.
type Logger struct {
	prefix string
	built  int
}

func NewLogger() *Logger { return &Logger{built: 1} }

func (l *Logger) SetPrefix(prefix string) { l.prefix = prefix }

func (l *Logger) Format(msg string) string { return l.prefix + msg }

// Banner uses its dependency while being built, not just stores it.
type Banner struct {
	text string
}

func NewBanner(l *Logger) *Banner { return &Banner{text: l.Format("started")} }

// Left and Right wire to each other: Left through its factory, Right through a
// method call.
type Left struct {
	n     int
	right *Right
}

type Right struct {
	n    int
	left *Left
}

func (r *Right) SetLeft(l *Left) { r.left = l }

// callLog records the order factories and method calls run in.
type callLog struct {
	entries []string
}

func (l *callLog) add(entry string) { l.entries = append(l.entries, entry) }

// Early pulls Late in from a method call, so Late is built after every factory
// the request itself needed.
type Early struct {
	log  *callLog
	late *Late
}

func (e *Early) UseLate(late *Late) { e.log.add("Early.UseLate"); e.late = late }

type Late struct {
	log *callLog
}

func (l *Late) Ready() { l.log.add("Late.Ready") }

func TestAFactoryDoesNotSeeMethodCallsOfItsDependencies(t *testing.T) {
	t.Parallel()

	c, err := di.New().
		Services(
			di.Svc(NewLogger).MethodCall((*Logger).SetPrefix, "app: "),
			di.Svc(NewBanner),
		).
		Build()
	require.NoError(t, err)

	banner, err := di.SvcByType[*Banner](c)
	require.NoError(t, err)
	require.Equal(t, "started", banner.text)

	logger, err := di.SvcByType[*Logger](c)
	require.NoError(t, err)
	require.Equal(t, "app: ", logger.prefix)
}

func TestWiringThatLoopsThroughAMethodCallBuildsOneOfEach(t *testing.T) {
	tests := []struct {
		name  string
		eager bool
		first func(t *testing.T, c di.Container)
	}{
		{
			name:  "the service the factory points from is asked for first",
			first: func(t *testing.T, c di.Container) { _, err := di.SvcByType[*Left](c); require.NoError(t, err) },
		},
		{
			name:  "the service the method call is on is asked for first",
			first: func(t *testing.T, c di.Container) { _, err := di.SvcByType[*Right](c); require.NoError(t, err) },
		},
		{
			name:  "both are built with the container",
			eager: true,
			first: func(t *testing.T, c di.Container) {},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var lefts, rights int

			var opts []di.BuilderOption
			if tt.eager {
				opts = append(opts, di.DefaultEager())
			}

			c, err := di.New(opts...).
				Services(
					di.Svc(func(r *Right) *Left { lefts++; return &Left{n: lefts, right: r} }),
					di.Svc(func() *Right { rights++; return &Right{n: rights} }).
						MethodCall((*Right).SetLeft),
				).
				Build()
			require.NoError(t, err)

			tt.first(t, c)

			left, err := di.SvcByType[*Left](c)
			require.NoError(t, err)
			right, err := di.SvcByType[*Right](c)
			require.NoError(t, err)

			require.Equal(t, 1, lefts)
			require.Equal(t, 1, rights)
			require.Same(t, right, left.right)
			require.Same(t, left, right.left)
		})
	}
}

func TestAMethodCallQueuesTheCallsOfWhatItBuilds(t *testing.T) {
	t.Parallel()

	log := new(callLog)

	c, err := di.New().
		Services(
			di.SvcVal(log),
			di.Svc(func(l *callLog) *Early { l.add("Early factory"); return &Early{log: l} }).
				MethodCall((*Early).UseLate),
			di.Svc(func(l *callLog) *Late { l.add("Late factory"); return &Late{log: l} }).
				MethodCall((*Late).Ready),
		).
		Build()
	require.NoError(t, err)

	early, err := di.SvcByType[*Early](c)
	require.NoError(t, err)
	require.NotNil(t, early.late)

	require.Equal(t, []string{
		"Early factory",
		"Late factory",
		"Early.UseLate",
		"Late.Ready",
	}, log.entries)
}

func TestAServiceThatIsNotSharedConfiguresEveryInstance(t *testing.T) {
	t.Parallel()

	c, err := di.New().
		Services(
			di.Svc(NewLogger).NotShared().MethodCall((*Logger).SetPrefix, "app: "),
		).
		Build()
	require.NoError(t, err)

	first, err := di.SvcByType[*Logger](c)
	require.NoError(t, err)
	second, err := di.SvcByType[*Logger](c)
	require.NoError(t, err)

	require.NotSame(t, first, second)
	require.Equal(t, "app: ", first.prefix)
	require.Equal(t, "app: ", second.prefix)
}

func TestAFactoryCycleIsReportedWhenTheServiceIsAskedFor(t *testing.T) {
	t.Parallel()

	var aRef, bRef, cRef di.SvcReference

	c, err := di.New(di.SkipCycleValidation()).
		Services(
			di.Svc(Echo[string], di.Ref(&bRef)).Bind(&aRef).Labels("echo-a"),
			di.Svc(Echo[string], di.Ref(&cRef)).Bind(&bRef).Labels("echo-b"),
			di.Svc(Echo[string], di.Ref(&aRef)).Bind(&cRef).Labels("echo-c"),
		).
		Build()
	require.NoError(t, err)

	_, err = di.SvcByRef[string](c, aRef)
	require.ErrorContains(t, err, "circular dependency: string (echo-a) -> string (echo-b) -> string (echo-c) -> string (echo-a)")
}

// Concurrent construction is synchronised no further than the instances map.
// Two goroutines can both build the same service, and one can be handed a
// service the other has published but not yet configured. What the map itself
// must never do is race.
func TestBuildingFromSeveralGoroutinesDoesNotRaceTheInstancesMap(t *testing.T) {
	t.Parallel()

	c, err := di.New().Services(di.Svc(NewLogger)).Build()
	require.NoError(t, err)

	const goroutines = 16

	errs := make([]error, goroutines)

	var wg sync.WaitGroup
	for i := range goroutines {
		wg.Go(func() {
			_, errs[i] = di.SvcByType[*Logger](c)
		})
	}
	wg.Wait()

	require.NoError(t, errors.Join(errs...))
}
