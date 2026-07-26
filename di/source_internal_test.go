package di

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The registering frame cannot be tested from in here: an internal test file
// compiles into package di, so its own frames are wiring frames and are walked
// past exactly as they should be. That half lives in the root package's
// external tests, which is also where a user's code sits relative to godi.

func TestPackageOf(t *testing.T) {
	tests := []struct {
		fn   string
		want string
	}{
		{"github.com/acme/app.NewServer", "github.com/acme/app"},
		{"github.com/acme/app.(*Server).Close", "github.com/acme/app"},
		{"github.com/acme/app.NewServer.func1", "github.com/acme/app"},
		{"main.main", "main"},
		{"github.com/michalkurzeja/godi/v2.Svc", "github.com/michalkurzeja/godi/v2"},
		{"github.com/michalkurzeja/godi/v2/di.NewServiceDefinition", "github.com/michalkurzeja/godi/v2/di"},
	}

	for _, tt := range tests {
		require.Equal(t, tt.want, packageOf(tt.fn), tt.fn)
	}
}

// Only godi's own wiring packages are walked past. Its examples and tests
// register services like anyone else, and they are the caller.
func TestOnlyTheWiringPackagesAreWalkedPast(t *testing.T) {
	tests := []struct {
		fn   string
		want bool
	}{
		{"github.com/michalkurzeja/godi/v2.Svc", true},
		{"github.com/michalkurzeja/godi/v2/di.NewServiceDefinition", true},
		{"github.com/michalkurzeja/godi/v2/extras.OverrideSvcArg", true},
		{"github.com/michalkurzeja/godi/v2/examples/graph.build", false},
		{"github.com/michalkurzeja/godi/v2/di_test.TestThing", false},
		{"github.com/michalkurzeja/godi/v2/graph/html.New", false},
		{"github.com/acme/app.wire", false},
		{"main.main", false},
	}

	for _, tt := range tests {
		require.Equal(t, tt.want, isWiring(tt.fn), tt.fn)
	}
}

func TestAnEmptySourceResolvesToNothing(t *testing.T) {
	require.Zero(t, source{}.Location())
}

func BenchmarkCaptureSource(b *testing.B) {
	for b.Loop() {
		captureSource()
	}
}

func BenchmarkResolveSource(b *testing.B) {
	s := captureSource()
	b.ResetTimer()
	for b.Loop() {
		s.Location()
	}
}
