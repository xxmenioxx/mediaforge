package models

import (
	"path/filepath"
	"strings"
)

// CanonicalPublicationPathKey returns the filesystem path identity used by
// lifecycle publication matching. Keep it aligned with filepath.Clean-based
// comparisons in the canonical asset lifecycle projection.
func CanonicalPublicationPathKey(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	clean := filepath.Clean(path)
	if clean == "." {
		return ""
	}
	return clean
}
