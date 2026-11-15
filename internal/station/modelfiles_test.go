// SPDX-License-Identifier: MIT

package station

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// allocated returns the octets f allocates.
func allocated(f func()) uint64 {
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	f()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

// fewOctets bounds what checking a file of the model may allocate: far less than the files
// the cases name.
const fewOctets = 16 << 20

// TestModelFileSizes: a files[].size is checked against the largest g70 file (4 294 967 295
// octets) without the content being made; the content is made once, by New.
func TestModelFileSizes(t *testing.T) {
	for _, c := range []struct {
		size uint64
		ok   bool
	}{
		{1, true},
		{math.MaxInt32, true}, // the largest int of a 32-bit platform
		{math.MaxUint32 - 1, true},
		{math.MaxUint32, true},
		{math.MaxUint32 + 1, false},
		{math.MaxInt64, false},
	} {
		t.Run(fmt.Sprint(c.size), func(t *testing.T) {
			m, err := Parse(fmt.Appendf(nil, "points: [{type: binary-input, index: 0, value: true}]\nfiles: [{name: /f, size: %d}]\n", c.size))
			if c.size > math.MaxInt {
				// An int of this platform does not hold the size: the model does not parse.
				if err == nil {
					t.Fatalf("a size of %d parsed into an int of %d bits", c.size, 32<<(^uint(0)>>63))
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var verr error
			if n := allocated(func() { verr = m.Validate() }); n > fewOctets {
				t.Errorf("Validate allocated %d octets", n)
			}
			switch {
			case c.ok && verr != nil:
				t.Errorf("a size of %d was refused: %v", c.size, verr)
			case !c.ok && (verr == nil || !strings.Contains(verr.Error(), "the maximum is 4294967295")):
				t.Errorf("a size of %d: %v, want the maximum", c.size, verr)
			}
		})
	}

	// New makes the repeating pattern of a size once, octet for octet as it always was.
	const size = 100_003
	m, err := Parse(fmt.Appendf(nil, "points: [{type: binary-input, index: 0, value: true}]\nfiles: [{name: /f, size: %d}]\n", size))
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	fs, err := newFileServer(m, nil, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	data := fs.files["/f"].data
	if len(data) != size {
		t.Fatalf("%d octets, want %d", len(data), size)
	}
	for i, b := range data {
		if b != byte('A'+i%26) {
			t.Fatalf("octet %d is %q, want %q", i, b, byte('A'+i%26))
		}
	}
	// New also refuses a size it was not checked for.
	for _, bad := range []FileSpec{{Name: "/n", Size: -1}} {
		if _, err := (&Model{}).fileContent(bad); err == nil {
			t.Errorf("fileContent(%+v) was accepted", bad)
		}
	}
}

// TestModelFilePaths: a files[].path must name a regular file; a directory is refused
// before it is read. TestModelFileSparse has a file that is too large.
func TestModelFilePaths(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "small.bin"), []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}
	model := func(p string) *Model {
		m, err := Parse([]byte("points: [{type: binary-input, index: 0, value: true}]\nfiles: [{name: /f, path: '" + p + "'}]\n"))
		if err != nil {
			t.Fatal(err)
		}
		m.dir = dir
		return m
	}
	if err := model("small.bin").Validate(); err != nil {
		t.Errorf("a small regular file: %v", err)
	}
	for _, p := range []string{"sub", filepath.Join(dir, "sub")} {
		if err := model(p).Validate(); err == nil || !strings.Contains(err.Error(), "is not a regular file") {
			t.Errorf("the directory %s: %v", p, err)
		}
	}

}

// TestModelFileLimit: a model file of more than 16 MiB is refused from its size, without
// being read.
func TestModelFileLimit(t *testing.T) {
	const head = "points: [{type: binary-input, index: 0, value: true}]\n#"
	file := filepath.Join(t.TempDir(), "model.yaml")
	for _, c := range []struct {
		size int
		ok   bool
	}{{maxModelSize - 1, true}, {maxModelSize, true}, {maxModelSize + 1, false}} {
		body := head + strings.Repeat("x", c.size-len(head)-1) + "\n"
		if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		var err error
		n := allocated(func() { _, err = Load(file) })
		switch {
		case c.ok && err != nil:
			t.Errorf("a model of %d octets: %v", c.size, err)
		case !c.ok && (err == nil || !strings.Contains(err.Error(), "at most 16777216 octets")):
			t.Errorf("a model of %d octets: %v, want the limit", c.size, err)
		case !c.ok && n > fewOctets:
			t.Errorf("a model of %d octets: Load allocated %d octets", c.size, n)
		}
	}
}
