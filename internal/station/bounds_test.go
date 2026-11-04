// SPDX-License-Identifier: MIT

package station

import (
	"context"
	"math"
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
