// SPDX-License-Identifier: MIT

//go:build !unix

package dnp

import "io/fs"

// loosePermissions reports nothing: the permission bits of Unix do not describe who may
// read a file on this system.
func loosePermissions(string) (fs.FileMode, bool) { return 0, false }
