// Package render holds the text handling shared by the graph encoders:
// shortening Go signatures for labels, wrapping them, and escaping them.
// It lives under graph/ so that it cannot leak back into the container engine.
package render

import (
	"strings"
	"unicode/utf8"

	"github.com/michalkurzeja/godi/v2/internal/util"
)

// Short drops the package paths from a Go signature, keeping the last segment
// of each: "github.com/acme/app/http.(*Server)" becomes "http.(*Server)".
// Names without a package path, such as "string" or "[]int", are returned as
// they are.
//
// Every path in the string goes, not just one. A signature can carry several -
// a generic instantiation names its type arguments, and a binding onto more
// than one implementation records them together - and shortening only the last
// would leave the rest with their paths still on.
func Short(signature string) string {
	var sb strings.Builder

	// Anything that is not part of a path is type syntax and has to stay:
	// "[]github.com/acme/app.T" must keep its brackets. So the string is walked
	// in runs, and only the runs that hold a path are cut.
	for i := 0; i < len(signature); {
		if !isPathByte(signature[i]) {
			sb.WriteByte(signature[i])
			i++
			continue
		}

		start := i
		for i < len(signature) && isPathByte(signature[i]) {
			i++
		}

		run := signature[start:i]
		if slash := strings.LastIndexByte(run, '/'); slash >= 0 {
			run = run[slash+1:]
		}
		sb.WriteString(run)
	}

	return sb.String()
}

// isPathByte reports whether b can appear in a Go import path, or in the
// qualified name that follows one. Import paths are ASCII, so scanning by byte
// is safe.
func isPathByte(b byte) bool {
	switch {
	case b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b >= '0' && b <= '9':
		return true
	default:
		// The slash among them, so a whole import path is one run.
		return strings.IndexByte("./-_~+", b) >= 0
	}
}

// Ellipsis shortens s to at most maxRunes runes, marking the cut with an
// ellipsis. A maxRunes of zero or less leaves s alone.
func Ellipsis(s string, maxRunes int) string {
	if _, cut := util.Truncate(s, maxRunes); !cut {
		return s // Fits as it is, ellipsis included.
	}

	// Leave room for the ellipsis, so the result still fits the budget.
	clipped, _ := util.Truncate(s, maxRunes-1)
	return clipped + "…"
}

// Wrap breaks s into lines of at most width runes, splitting on the separators
// common in Go signatures so that a wrapped type stays readable.
func Wrap(s string, width int) []string {
	if width <= 0 || utf8.RuneCountInString(s) <= width {
		return []string{s}
	}

	var (
		lines []string
		line  strings.Builder
		n     int
	)
	for _, chunk := range chunks(s) {
		size := utf8.RuneCountInString(chunk)
		if n > 0 && n+size > width {
			lines = append(lines, line.String())
			line.Reset()
			n = 0
		}
		line.WriteString(chunk)
		n += size
	}
	if line.Len() > 0 {
		lines = append(lines, line.String())
	}
	return lines
}

// chunks splits s after each separator, so that a break never lands mid-identifier.
func chunks(s string) []string {
	var (
		out   []string
		start int
	)
	for i, r := range s {
		if strings.ContainsRune(",]) ", r) {
			out = append(out, s[start:i+utf8.RuneLen(r)])
			start = i + utf8.RuneLen(r)
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}
