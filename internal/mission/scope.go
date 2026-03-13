package mission

import (
	"path/filepath"
	"strings"
)

// pathsOverlap returns true if two path globs could refer to overlapping files.
// It uses a conservative heuristic: if one path is a prefix of the other
// (ignoring glob metacharacters), they overlap.
func pathsOverlap(a, b string) bool {
	// Normalize paths.
	a = filepath.Clean(a)
	b = filepath.Clean(b)

	// Strip trailing glob patterns for prefix comparison.
	aBase := stripGlob(a)
	bBase := stripGlob(b)

	// Two paths overlap if one is a prefix of the other.
	return strings.HasPrefix(aBase, bBase) || strings.HasPrefix(bBase, aBase)
}

// stripGlob removes trailing glob segments (**, *, etc.) from a path
// and returns the directory prefix.
func stripGlob(p string) string {
	parts := strings.Split(p, "/")
	var clean []string
	for _, part := range parts {
		if strings.ContainsAny(part, "*?[") {
			break
		}
		clean = append(clean, part)
	}
	if len(clean) == 0 {
		return "."
	}
	return strings.Join(clean, "/")
}
