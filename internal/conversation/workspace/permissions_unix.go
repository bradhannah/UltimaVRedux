//go:build !windows

package workspace

import "io/fs"

// sharedWithOthers reports whether group or other users may access the path.
func sharedWithOthers(mode fs.FileMode) bool {
	return mode.Perm()&0o077 != 0
}
