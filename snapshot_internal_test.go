package di

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// A path with a not-yet-created directory component looks the same to os.Stat
// as one that is genuinely meant to be a file: both fail with ErrNotExist.
// There is no other signal for which the caller meant, so this stays the file
// fallback it always was.
func TestCreateFileTreatsANotYetExistingPathAsAFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "graph.json")
	w := snapshotWriter{path: path}

	f, err := w.createFile()
	require.NoError(t, err)
	require.NoError(t, f.Close())
	require.Equal(t, path, f.Name())
}

// A stat error that is not "nothing there yet" - permission denied, a path
// component that is not a directory, and so on - is not a signal to guess
// "must be a file" from. Returning it directly beats a second, more confusing
// failure at OpenFile.
func TestCreateFileSurfacesAnUnexpectedStatErrorRatherThanGuessing(t *testing.T) {
	t.Parallel()

	// A regular file cannot have children, so treating it as a directory
	// component fails with something other than ErrNotExist (ENOTDIR).
	notADir := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(notADir, []byte("x"), 0o600))

	w := snapshotWriter{path: filepath.Join(notADir, "graph.json")}

	_, err := w.createFile()
	require.Error(t, err)
	require.False(t, errors.Is(err, os.ErrNotExist), "this is not the not-yet-created-directory case")
}
