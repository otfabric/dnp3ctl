// SPDX-License-Identifier: MIT

package station

// SetMaxFileSize lowers the largest file a master may write, for the test, and
// returns the function that restores it.
func SetMaxFileSize(n int64) (restore func()) {
	old := maxFileSize
	maxFileSize = n
	return func() { maxFileSize = old }
}
