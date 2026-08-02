package render_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michalkurzeja/godi/v2/graph/internal/render"
)

func TestShort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"pointer to a named type", "github.com/acme/app/http.(*Server)", "http.(*Server)"},
		{"named type", "github.com/acme/app/http.Server", "http.Server"},
		{"factory function", "github.com/acme/app/http.NewServer", "http.NewServer"},
		{"builtin", "string", "string"},
		{"slice of builtin", "[]int", "[]int"},
		{"slice of pointers to a type", "[]github.com/acme/app/http.(*Server)", "[]http.(*Server)"},
		{"slice of named types", "[]github.com/acme/app/http.Server", "[]http.Server"},
		{"map with a qualified value", "map[string]github.com/acme/app.Entry", "map[string]app.Entry"},
		{"directional channel of a qualified type", "chan<- github.com/acme/app.Event", "chan<- app.Event"},
		{"empty interface", "interface {}", "interface {}"},
		{"directional channel", "chan<- int", "chan<- int"},
		{"already short", "http.Server", "http.Server"},
		// The last slash in these is inside the type arguments, so shortening
		// only around it leaves the name itself qualified.
		{"generic with qualified arguments",
			"github.com/acme/app.Handler[github.com/acme/app.Request, github.com/acme/app.Response]",
			"app.Handler[app.Request, app.Response]"},
		{"generic over a builtin", "github.com/acme/app.Slot[string]", "app.Slot[string]"},
		{"several signatures at once", "github.com/acme/a.A, github.com/acme/b.B", "a.A, b.B"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, render.Short(tt.in))
		})
	}
}

func TestEllipsis(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		max  int
		want string
	}{
		{"under the limit", "hello", 10, "hello"},
		{"exactly at the limit", "hello", 5, "hello"},
		{"over the limit, ellipsis included in the budget", "hello world", 5, "hell…"},
		{"no limit", "hello world", 0, "hello world"},
		{"cuts on rune boundaries", "wysoką jakość", 7, "wysoką…"},
		{"no room for content, only the mark", "hello", 1, "…"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := render.Ellipsis(tt.in, tt.max)
			require.Equal(t, tt.want, got)
			if tt.max > 0 {
				require.LessOrEqual(t, len([]rune(got)), max(tt.max, len([]rune(tt.in))))
			}
		})
	}
}

func TestWrap(t *testing.T) {
	t.Parallel()

	t.Run("leaves short input alone", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, []string{"http.Server"}, render.Wrap("http.Server", 40))
	})

	t.Run("breaks a generic signature after separators", func(t *testing.T) {
		t.Parallel()

		lines := render.Wrap("map[string]cache.Entry[app.Key, app.Value]", 20)
		require.Greater(t, len(lines), 1)
		for _, line := range lines {
			require.LessOrEqual(t, len([]rune(line)), 20+len("app.Value]"))
		}
		require.Equal(t, "map[string]cache.Entry[app.Key, app.Value]", join(lines))
	})

	t.Run("keeps an unbreakable run in one piece", func(t *testing.T) {
		t.Parallel()

		lines := render.Wrap("aVeryLongIdentifierWithNoSeparators", 10)
		require.Equal(t, []string{"aVeryLongIdentifierWithNoSeparators"}, lines)
	})
}

func join(lines []string) string {
	return strings.Join(lines, "")
}

func TestPackage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"a qualified type", "github.com/acme/app/http.(*Server)", "github.com/acme/app/http"},
		{"a factory", "github.com/acme/app.NewServer", "github.com/acme/app"},
		{"the standard library", "time.Duration", "time"},
		{"package main", "main.Config", "main"},
		{"a slice", "[]github.com/acme/app.T", "github.com/acme/app"},
		// The first qualified name is the type. The ones after it are its type
		// arguments, which may live anywhere.
		{"a generic", "github.com/acme/app.Handler[github.com/other/x.Req]", "github.com/acme/app"},
		{"a map, whose key comes first in the text", "map[string]github.com/acme/app.Entry", "github.com/acme/app"},
		{"a builtin", "string", ""},
		{"a bare func type", "func() error", ""},
		{"empty", "", ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, test.want, render.Package(test.in))
		})
	}
}
