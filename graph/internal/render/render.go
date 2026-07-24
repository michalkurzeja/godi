// Package render holds the text handling shared by the graph encoders:
// shortening Go signatures for labels, wrapping them, and escaping them.
// It lives under graph/ so that it cannot leak back into the container engine.
package render

import (
	"strings"
	"unicode/utf8"

	"github.com/michalkurzeja/godi/v2/internal/util"
)

// Short drops the package path from a fully qualified Go signature, keeping the
// last path segment: "github.com/acme/app/http.(*Server)" becomes "http.(*Server)".
// Names without a package path, such as "string" or "[]int", are returned as they are.
func Short(signature string) string {
	i := strings.LastIndexByte(signature, '/')
	if i < 0 {
		return signature
	}

	// Cut out only the import path itself. Anything in front of it is type
	// syntax and has to stay: "[]github.com/acme/app.T" must keep its "[]".
	start := i
	for start > 0 && isPathByte(signature[start-1]) {
		start--
	}
	return signature[:start] + signature[i+1:]
}

// isPathByte reports whether b can appear in a Go import path. Import paths are
// ASCII, so scanning by byte is safe.
func isPathByte(b byte) bool {
	switch {
	case b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b >= '0' && b <= '9':
		return true
	default:
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
