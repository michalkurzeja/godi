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
