// SPDX-License-Identifier: MIT

//go:build unix

package station

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestModelFileFIFO: a files[].path that names a FIFO is refused without opening it, which
// would wait for a writer.
func TestModelFileFIFO(t *testing.T) {
	dir := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(dir, "fifo"), 0o600); err != nil {
		t.Fatalf("a FIFO: %v", err)
	}
	m, err := Parse([]byte("points: [{type: binary-input, index: 0, value: true}]\nfiles: [{name: /f, path: fifo}]\n"))
	if err != nil {
		t.Fatal(err)
	}
	m.dir = dir
	done := make(chan error, 1)
	go func() { done <- m.Validate() }()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "fifo is not a regular file") {
			t.Errorf("a FIFO: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Validate still waits on the FIFO after 10 s")
	}
}

// TestModelFileSparse: a files[].path of a file one octet past the largest g70 file is
// refused from its size, unread. The file is sparse, which the file systems of unix
// platforms make of a truncated file (NTFS needs FSCTL_SET_SPARSE).
func TestModelFileSparse(t *testing.T) {
	dir := t.TempDir()
	big := filepath.Join(dir, "big.bin")
	if err := os.WriteFile(big, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(big, math.MaxUint32+1); err != nil {
		t.Fatalf("a sparse file of 4 GiB: %v", err)
	}
	for _, p := range []string{"big.bin", big} {
		m, err := Parse([]byte("points: [{type: binary-input, index: 0, value: true}]\nfiles: [{name: /f, path: '" + p + "'}]\n"))
		if err != nil {
			t.Fatal(err)
		}
		m.dir = dir
		if n := allocated(func() { err = m.Validate() }); n > fewOctets {
			t.Errorf("%s: Validate allocated %d octets", p, n)
		}
		if err == nil || !strings.Contains(err.Error(), "4294967296 octets: the maximum is 4294967295") {
			t.Errorf("%s: %v", p, err)
		}
		if n := allocated(func() { _, err = m.fileContent(m.Files[0]) }); n > fewOctets || err == nil {
			t.Errorf("%s: fileContent allocated %d octets: %v", p, n, err)
		}
	}
}
