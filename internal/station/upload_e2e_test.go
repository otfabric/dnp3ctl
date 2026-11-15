// SPDX-License-Identifier: MIT

package station_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/otfabric/go-dnp3/client"
	"github.com/otfabric/go-dnp3/link"

	"github.com/otfabric/dnp3ctl/internal/station"
	"github.com/otfabric/dnp3ctl/internal/stationtest"
)

// TestUploadPastLimitOverDNP3: a master that writes more than the outstation can hold
// gets an error from the outstation, and the outstation keeps no part of the file.
func TestUploadPastLimitOverDNP3(t *testing.T) {
	defer station.SetMaxFileSize(1000)()
	st := stationtest.Start(t, stationtest.Model)
	c, err := client.New(st.Addr(), client.WithAddresses(1, link.Address(10)), client.WithResponseTimeout(2*time.Second),
		client.WithStartup(client.Startup{}), client.WithTimeSync(client.TimeSyncNone))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := c.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	const bigSize, smallSize = 3000, 900
	data := bytes.Repeat([]byte{0x5A}, bigSize)
	if _, err := c.WriteFile(ctx, "/big.bin", bytes.NewReader(data), bigSize, client.FileOptions{MaxBlockSize: 256}); err == nil {
		t.Fatal("a write past the limit succeeded")
	} else {
		t.Logf("write: %v", err)
	}
	if _, err := c.FileInfo(ctx, "/big.bin"); err == nil {
		t.Error("the outstation kept part of a refused file")
	}
	// A file within the limit is still taken.
	small := bytes.Repeat([]byte{0xA5}, smallSize)
	if _, err := c.WriteFile(ctx, "/small.bin", bytes.NewReader(small), smallSize, client.FileOptions{MaxBlockSize: 256}); err != nil {
		t.Fatalf("a write within the limit: %v", err)
	}
	var got bytes.Buffer
	if _, err := c.ReadFile(ctx, "/small.bin", &got, client.FileOptions{}); err != nil || !bytes.Equal(got.Bytes(), small) {
		t.Fatalf("read back: %d octets, %v", got.Len(), err)
	}
}

// TestUploadTotalOverDNP3: the files masters write hold at most uploads.max_bytes together; a
// write past it gets an error and leaves no file, and a delete makes room again.
func TestUploadTotalOverDNP3(t *testing.T) {
	model := strings.Replace(stationtest.Model, "uploads: {accept: true}", "uploads: {accept: true, max_bytes: 2000}", 1)
	if model == stationtest.Model {
		t.Fatal("the test model has no uploads: {accept: true}")
	}
	st := stationtest.Start(t, model)
	c, err := client.New(st.Addr(), client.WithAddresses(1, link.Address(10)), client.WithResponseTimeout(2*time.Second),
		client.WithStartup(client.Startup{}), client.WithTimeSync(client.TimeSyncNone))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := c.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	put := func(name string, n int) error {
		_, err := c.WriteFile(ctx, name, bytes.NewReader(bytes.Repeat([]byte{0x5A}, n)), uint32(n), client.FileOptions{MaxBlockSize: 256})
		return err
	}
	if err := put("/one.bin", 1500); err != nil {
		t.Fatalf("1500 of 2000 octets: %v", err)
	}
	if err := put("/two.bin", 1000); err == nil {
		t.Fatal("2500 of 2000 octets were taken")
	} else {
		t.Logf("write: %v", err)
	}
	if _, err := c.FileInfo(ctx, "/two.bin"); err == nil {
		t.Error("the outstation kept part of a refused file")
	}
	if err := c.DeleteFile(ctx, "/one.bin", client.FileOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := put("/two.bin", 1000); err != nil {
		t.Fatalf("1000 octets after the delete: %v", err)
	}
	var got bytes.Buffer
	if _, err := c.ReadFile(ctx, "/two.bin", &got, client.FileOptions{}); err != nil || got.Len() != 1000 {
		t.Fatalf("read back: %d octets, %v", got.Len(), err)
	}
}
