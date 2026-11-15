// SPDX-License-Identifier: MIT

package station

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/otfabric/go-dnp3/app"
	"github.com/otfabric/go-dnp3/server"
)

func TestSize32(t *testing.T) {
	for _, c := range []struct {
		n    int
		want uint32
	}{{-1, 0}, {0, 0}, {5000, 5000}, {math.MaxInt32, math.MaxInt32}} {
		if got := size32(c.n); got != c.want {
			t.Errorf("size32(%d) = %d, want %d", c.n, got, c.want)
		}
	}
	// MaxInt is past 4 GiB on 64-bit platforms and clamps there; on 32-bit ones it fits.
	if got, want := size32(math.MaxInt), uint32(min(uint64(math.MaxInt), math.MaxUint32)); got != want {
		t.Errorf("size32(MaxInt) = %d, want %d", got, want)
	}
}

// TestUploadSizeLimit: a master cannot write a file past the largest size of g70;
// the write fails, instead of the size of the stored file wrapping, and an append
// counts the octets already there.
func TestUploadSizeLimit(t *testing.T) {
	defer SetMaxFileSize(10)()
	m, err := Parse([]byte("outstation: {}\npoints: [{type: binary-input, index: 0, value: true}]\nfiles: [{name: /a, content: abcdef}]\nuploads: {accept: true}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	l := &eventLog{}
	fs, err := newFileServer(m, l.log, func() time.Time { return time.Unix(0, 0) })
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	w, st := fs.Open(ctx, server.FileRequest{Name: "/b", Mode: app.FileModeWrite})
	if st != app.FileSuccess {
		t.Fatal(st)
	}
	if _, err := w.Write([]byte("0123456789")); err != nil {
		t.Fatalf("a write up to the limit: %v", err)
	}
	if _, err := w.Write([]byte("x")); err == nil || !strings.Contains(err.Error(), "largest size") {
		t.Fatalf("a write past the limit: %v", err)
	}
	if w.Size() != 10 {
		t.Errorf("size %d after the refused write, want 10", w.Size())
	}
	_ = w.Close(true)

	a, st := fs.Open(ctx, server.FileRequest{Name: "/a", Mode: app.FileModeAppend})
	if st != app.FileSuccess {
		t.Fatal(st)
	}
	if _, err := a.Write([]byte("12345")); err == nil {
		t.Fatal("an append past the limit was accepted")
	}
	if _, err := a.Write([]byte("1234")); err != nil {
		t.Fatalf("an append up to the limit: %v", err)
	}
	if err := a.Close(false); err != nil {
		t.Fatal(err)
	}
	if d, _ := fs.Info(ctx, "/a"); d.Size != 10 {
		t.Errorf("/a: %d octets, want 10", d.Size)
	}
}

// TestWritableVariationsAboveByte: a writable variation past 255 is refused by
// validation, and never truncated into one that exists.
func TestWritableVariationsAboveByte(t *testing.T) {
	// 257 would truncate to 1, an attribute of the set.
	m, err := Parse([]byte("outstation: {attribute_sets: [{set: 1, attributes: {1: a}, writable: [257]}]}\npoints: [{type: binary-input, index: 0, value: true}]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Validate(); err == nil || !strings.Contains(err.Error(), "writable variation 257") {
		t.Fatalf("Validate: %v", err)
	}
	if got := (AttributeSetSpec{Writable: []uint16{1, 257, 2}}).writable(); len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Errorf("writable() = %v, want [1 2]", got)
	}
}

// uploadServer is the file server of a model with uploads, and a check of its counts: the
// files masters wrote and their octets are what the server counts, and with the transfers
// in progress within the limits.
func uploadServer(t *testing.T, model string) (*fileServer, func(step string)) {
	t.Helper()
	m, err := Parse([]byte(model))
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	l := &eventLog{}
	fs, err := newFileServer(m, l.log, func() time.Time { return time.Unix(0, 0) })
	if err != nil {
		t.Fatal(err)
	}
	return fs, func(step string) {
		t.Helper()
		fs.mu.Lock()
		defer fs.mu.Unlock()
		var octets int64
		files := 0
		for _, e := range fs.files {
			if e.uploaded {
				octets += int64(len(e.data))
				files++
			}
		}
		switch {
		case octets != fs.held || files != fs.uploads:
			t.Errorf("%s: %d uploads of %d octets, counted %d of %d", step, files, octets, fs.uploads, fs.held)
		case octets+fs.sending > fs.maxBytes || files+fs.adding > fs.maxFiles:
			t.Errorf("%s: %d uploads and %d transfers adding one hold %d octets and %d in transfer: past %d files or %d octets",
				step, files, fs.adding, octets, fs.sending, fs.maxFiles, fs.maxBytes)
		}
	}
}

// TestUploadLimits: the files masters write count toward uploads.max_files and
// uploads.max_bytes, the transfers in progress included, an append as its new size; the
// files of the model do not count; a delete makes room again.
func TestUploadLimits(t *testing.T) {
	fs, check := uploadServer(t, "outstation: {}\npoints: [{type: binary-input, index: 0, value: true}]\n"+
		"files: [{name: /model.txt, content: abcdefghij}]\nuploads: {accept: true, max_files: 3, max_bytes: 10}\n")
	ctx := context.Background()
	open := func(name string, mode app.FileMode) server.File {
		t.Helper()
		f, st := fs.Open(ctx, server.FileRequest{Name: name, Mode: mode})
		if st != app.FileSuccess {
			t.Fatalf("open %s: %v", name, st)
		}
		return f
	}
	write := func(f server.File, data string, ok bool) {
		t.Helper()
		if _, err := f.Write([]byte(data)); (err == nil) != ok {
			t.Errorf("write of %d octets: %v, want accepted %v", len(data), err, ok)
		}
	}
	content := func(name string) string {
		fs.mu.Lock()
		defer fs.mu.Unlock()
		if e, ok := fs.files[name]; ok {
			return string(e.data)
		}
		return "<none>"
	}

	a := open("/a", app.FileModeWrite)
	write(a, "1234", true)
	_ = a.Close(false)
	check("/a")
	b := open("/b", app.FileModeWrite)
	write(b, "12", true)
	_ = b.Close(false)
	check("/b")
	// The third file, the most there may be, up to the last octet.
	c := open("/c", app.FileModeWrite)
	write(c, "12345", false) // 4 + 2 + 5 = 11
	write(c, "1234", true)   // 10
	check("/c in progress")
	// A fourth name, while the third is in progress: refused at the open.
	if _, st := fs.Open(ctx, server.FileRequest{Name: "/d", Mode: app.FileModeWrite}); st != app.FilePermissionDenied {
		t.Errorf("a fourth file while the third is written: %v", st)
	}
	_ = c.Close(false)
	check("/c")
	if _, st := fs.Open(ctx, server.FileRequest{Name: "/d", Mode: app.FileModeAppend}); st != app.FilePermissionDenied {
		t.Errorf("a fourth file: %v", st)
	}
	// Writing a file again adds no file, but its octets count until it is replaced.
	again := open("/a", app.FileModeWrite)
	write(again, "x", false)
	_ = again.Close(true)
	check("/a again, aborted")
	if got := content("/a"); got != "1234" {
		t.Errorf("/a after an aborted write: %q", got)
	}
	// A delete makes room; an append counts as the octets it adds.
	if st := fs.Delete(ctx, "/a", 0); st != app.FileSuccess {
		t.Fatal(st)
	}
	check("delete /a")
	more := open("/b", app.FileModeAppend)
	write(more, "12345", false) // 2 + 4 + 5 = 11
	write(more, "1234", true)   // 10
	_ = more.Close(false)
	check("append to /b")
	if got := content("/b"); got != "121234" {
		t.Errorf("/b after the append: %q", got)
	}
	// The model's own file does not count, until a master writes it: then it is an upload.
	if got := content("/model.txt"); got != "abcdefghij" {
		t.Errorf("/model.txt: %q", got)
	}
	over := open("/model.txt", app.FileModeWrite)
	write(over, "z", false)
	_ = over.Close(true)
	check("/model.txt")
	if got := content("/model.txt"); got != "abcdefghij" {
		t.Errorf("/model.txt after a refused write: %q", got)
	}
	// A transfer that ends in an error of the upload directory frees what it counted.
	fs.dir = filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(fs.dir, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if st := fs.Delete(ctx, "/c", 0); st != app.FileSuccess {
		t.Fatal(st)
	}
	e := open("/e", app.FileModeWrite)
	write(e, "1", true)
	if err := e.Close(false); err == nil {
		t.Error("an upload below a file was stored")
	}
	check("/e not stored")
	fs.mu.Lock()
	if fs.sending != 0 || fs.adding != 0 {
		t.Errorf("after the transfers: %d octets and %d files in transfer", fs.sending, fs.adding)
	}
	fs.mu.Unlock()
}

// TestUploadDefaultLimits: without max_files and max_bytes a model takes 64 files of
// masters, of 64 MiB together.
func TestUploadDefaultLimits(t *testing.T) {
	fs, check := uploadServer(t, DemoYAML)
	ctx := context.Background()
	put := func(name string, n int) (app.FileStatus, error) {
		f, st := fs.Open(ctx, server.FileRequest{Name: name, Mode: app.FileModeWrite})
		if st != app.FileSuccess {
			return st, nil
		}
		if _, err := f.Write(make([]byte, n)); err != nil {
			_ = f.Close(true)
			return st, err
		}
		return st, f.Close(false)
	}
	for i := range 64 {
		if st, err := put(fmt.Sprintf("/up/%d", i), 1); st != app.FileSuccess || err != nil {
			t.Fatalf("file %d: %v, %v", i+1, st, err)
		}
	}
	check("64 files")
	if st, _ := put("/up/64", 1); st != app.FilePermissionDenied {
		t.Errorf("file 65: %v, want PERMISSION_DENIED", st)
	}
	if st := fs.Delete(ctx, "/up/63", 0); st != app.FileSuccess {
		t.Fatal(st)
	}
	// 63 files of one octet and one of the rest: 64 MiB.
	if st, err := put("/up/big", 64<<20-63+1); err == nil {
		t.Errorf("a file one octet past 64 MiB together: %v", st)
	}
	if st, err := put("/up/big", 64<<20-63); st != app.FileSuccess || err != nil {
		t.Errorf("a file up to 64 MiB together: %v, %v", st, err)
	}
	check("64 MiB")
}
