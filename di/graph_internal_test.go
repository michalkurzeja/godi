package di

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The source root is only worth recording when it actually shortens the paths.
// Trimming is done a component at a time, so two sibling directories that share
// a prefix - app and apple - must not be mistaken for one.
func TestTheSourceRootIsTheDirectoryEveryPathShares(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		paths []string
		want  string
	}{
		{"nothing to share", nil, ""},
		{"one path is its own directory", []string{"/home/u/app/wire.go"}, "/home/u/app"},
		{"the directory two files share", []string{"/home/u/app/wire.go", "/home/u/app/main.go"}, "/home/u/app"},
		{"a whole component, not a prefix of one", []string{"/home/u/app/x.go", "/home/u/apple/y.go"}, "/home/u"},
		{"the filesystem root is no help", []string{"/app/x.go", "/lib/y.go"}, ""},
		{"nor is the current directory", []string{"wire.go", "main.go"}, ""},
		{"a relative path and an absolute one share nothing", []string{"wire.go", "/home/u/app/x.go"}, ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, test.want, commonDir(test.paths))
		})
	}
}
