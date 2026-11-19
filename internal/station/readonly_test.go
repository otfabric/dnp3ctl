// SPDX-License-Identifier: MIT

package station

import (
	"context"
	"errors"
	"net"
	"slices"
	"testing"
	"time"

	dnp3 "github.com/otfabric/go-dnp3"
	"github.com/otfabric/go-dnp3/app"
	"github.com/otfabric/go-dnp3/client"
	"github.com/otfabric/go-dnp3/server"
)

// TestReadOnlyOctetString: a master writes two octet strings in one WRITE, one of them
// read_only. The outstation answers with IIN2.2 (IEEE 1815-2012 4.5.11); the read-only
// point keeps its value and makes no event, the other is stored with one g111 event, and
// both writes are logged, the refused one as refused.
func TestReadOnlyOctetString(t *testing.T) {
	st, l := newStation(t, `outstation: {}
points:
  - {type: octet-string, index: 0, value: fixed, class: 1, read_only: true}
  - {type: octet-string, index: 1, value: old, class: 1}
`)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = st.Serve(ln) }()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, err := client.Dial(ctx, ln.Addr().String(), client.WithStartup(client.Startup{}),
		client.WithAutoIntegrity(false), client.WithTimeSync(client.TimeSyncNone))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()

	_, err = c.Write(ctx,
		app.Ranged(app.GV(110, 3), 0, app.OctetString{Data: []byte("new")}),
		app.Ranged(app.GV(110, 3), 1, app.OctetString{Data: []byte("new")}))
	var ie *dnp3.IINError
	if !errors.As(err, &ie) || !ie.IIN.Has(app.IINParameterError) {
		t.Fatalf("WRITE of a read-only octet string: %v, want IIN2.2", err)
	}
	db := st.Server().Database()
	for i, want := range []string{"fixed", "new"} {
		v, _ := db.Get(server.OctetString, uint32(i))
		if got := string(v.(app.OctetString).Data); got != want {
			t.Errorf("octet string %d is %q, want %q", i, got, want)
		}
	}
	res, err := c.EventPoll(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	var events []uint32
	for _, m := range res.Measurements() {
		if m.GV.Group() == 111 {
			events = append(events, m.Index)
		}
	}
	if !slices.Equal(events, []uint32{1}) {
		t.Errorf("octet string events of %v, want [1]", events)
	}
	if !l.has("write", "refused: read-only") {
		t.Error("the refused write was not logged as refused")
	}
}
