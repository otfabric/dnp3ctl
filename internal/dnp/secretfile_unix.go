// SPDX-License-Identifier: MIT

//go:build unix

package dnp

import (
	"io/fs"
	"os"
)

// loosePermissions returns the permissions of path when its group or others have any.
func loosePermissions(path string) (fs.FileMode, bool) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return 0, false
	}
	perm := info.Mode().Perm()
	return perm, perm&0o077 != 0
}
