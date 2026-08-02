package util_test

import (
	"reflect"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"

	"github.com/michalkurzeja/godi/v2/internal/util"
)

func TestFuncName(t *testing.T) {
	tests := []struct {
		name      string
		fn        any
		wantFull  string
		wantShort string
	}{
		{
			name:      "myFunc",
			fn:        myFunc,
			wantFull:  "github.com/michalkurzeja/godi/v2/internal/util_test.myFunc",
			wantShort: "myFunc",
		},
		{
			name:      "nil",
			fn:        nil,
			wantFull:  "<not a func>",
			wantShort: "<not a func>",
		},
		{
			name:      "anonymous func",
			fn:        func() int { return 0 },
			wantFull:  "github.com/michalkurzeja/godi/v2/internal/util_test.TestFuncName.func1",
			wantShort: "func1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.wantFull, util.FuncName(reflect.ValueOf(tt.fn)))
			require.Equal(t, tt.wantShort, util.FuncNameShort(reflect.ValueOf(tt.fn)))
		})
	}
}

func myFunc() {}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name          string
		in            string
		max           int
		want          string
		wantTruncated bool
	}{
		{"under the limit", "hello", 10, "hello", false},
		{"exactly at the limit", "hello", 5, "hello", false},
		{"over the limit", "hello world", 5, "hello", true},
		{"no limit", "hello", 0, "hello", false},
		{"negative limit", "hello", -1, "hello", false},
		{"cuts on rune boundaries, never mid-character", "wysoką jakość", 6, "wysoką", true},
		{"multi-byte throughout", "日本語テキスト", 3, "日本語", true},
		{"empty", "", 5, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, truncated := util.Truncate(tt.in, tt.max)
			require.Equal(t, tt.want, got)
			require.Equal(t, tt.wantTruncated, truncated)
			require.True(t, utf8.ValidString(got), "truncation must not split a rune")
		})
	}
}

func TestSignature(t *testing.T) {
	type named struct{}

	tests := []struct {
		name string
		typ  reflect.Type
		want string
	}{
		{"named type", reflect.TypeFor[named](), "github.com/michalkurzeja/godi/v2/internal/util_test.named"},
		{"pointer to a named type", reflect.TypeFor[*named](), "github.com/michalkurzeja/godi/v2/internal/util_test.(*named)"},
		{"builtin", reflect.TypeFor[int](), "int"},
		// A pointer to something unnamed is still a pointer.
		{"pointer to a builtin", reflect.TypeFor[*int](), "*int"},
		{"pointer to a slice", reflect.TypeFor[*[]string](), "*[]string"},
		{"slice", reflect.TypeFor[[]int](), "[]int"},
		{"map", reflect.TypeFor[map[string]int](), "map[string]int"},
		{"channel", reflect.TypeFor[chan int](), "chan int"},
		{"func", reflect.TypeFor[func(int) string](), "func(int) string"},
		{"nil", nil, "<nil>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, util.Signature(tt.typ))
		})
	}
}
