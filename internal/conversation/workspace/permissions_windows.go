//go:build windows

package workspace

import "io/fs"

// sharedWithOthers cannot inspect Windows access-control lists: Go reports every
// path there as group and world accessible. Privacy on Windows therefore relies on
// the per-user profile ACLs of the workspace location instead of mode bits.
func sharedWithOthers(_ fs.FileMode) bool {
	return false
}
