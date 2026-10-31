// SPDX-License-Identifier: MIT

package dnp

import (
	"io"

	serial "github.com/otfabric/go-serial"
)

// SetOpenSerial replaces how serial ports are opened, for tests, and returns a function
// that restores it.
func SetOpenSerial(f func(*serial.Config) (io.ReadWriteCloser, error)) (restore func()) {
	prev := openSerial
	openSerial = f
	return func() { openSerial = prev }
}
