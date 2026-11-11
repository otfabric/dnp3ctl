// SPDX-License-Identifier: MIT

package station_test

import (
	"bytes"
	"context"
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
	data := bytes.Repeat([]byte{0x5A}, 3000)
	if _, err := c.WriteFile(ctx, "/big.bin", bytes.NewReader(data), uint32(len(data)), client.FileOptions{MaxBlockSize: 256}); err == nil {
		t.Fatal("a write past the limit succeeded")
	} else {
		t.Logf("write: %v", err)
	}
	if _, err := c.FileInfo(ctx, "/big.bin"); err == nil {
		t.Error("the outstation kept part of a refused file")
	}
	// A file within the limit is still taken.
	small := bytes.Repeat([]byte{0xA5}, 900)
	if _, err := c.WriteFile(ctx, "/small.bin", bytes.NewReader(small), uint32(len(small)), client.FileOptions{MaxBlockSize: 256}); err != nil {
		t.Fatalf("a write within the limit: %v", err)
	}
	var got bytes.Buffer
	if _, err := c.ReadFile(ctx, "/small.bin", &got, client.FileOptions{}); err != nil || !bytes.Equal(got.Bytes(), small) {
		t.Fatalf("read back: %d octets, %v", got.Len(), err)
	}
}
