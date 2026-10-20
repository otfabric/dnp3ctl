package dnp_test

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	serial "github.com/otfabric/go-serial"

	"github.com/otfabric/dnp3ctl/internal/config"
	"github.com/otfabric/dnp3ctl/internal/dnp"
	"github.com/otfabric/dnp3ctl/internal/stationtest"
)

// A serial outstation is reached through the stream opener: here a pipe to the simulated
// outstation stands in for the port.
func TestSerial(t *testing.T) {
	st := stationtest.Start(t, stationtest.Model)
	var opened *serial.Config
	restore := dnp.SetOpenSerial(func(c *serial.Config) (io.ReadWriteCloser, error) {
		opened = c
		a, b := net.Pipe()
		go func() { _ = st.ServeConn(b, nil) }()
		return a, nil
	})
	defer restore()

	cfg := &config.ReadConfig{
		ConnConfig: config.ConnConfig{
			URL:           "serial:///dev/ttyFAKE0",
			SerialConfig:  config.SerialConfig{Baud: 19200, Parity: "even"},
			AddressConfig: config.AddressConfig{Master: 1, Outstation: 10},
		},
		Objects: "g1v2",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := dnp.Read(ctx, cfg, nil)
	if err != nil || res.Count != 2 || res.Target != "serial:///dev/ttyFAKE0" {
		t.Fatalf("read over serial: %+v, %v", res, err)
	}
	if opened == nil || opened.Address != "/dev/ttyFAKE0" || opened.BaudRate != 19200 || opened.Parity != serial.ParityEven {
		t.Errorf("port opened with %+v", opened)
	}

	dnp.SetOpenSerial(func(*serial.Config) (io.ReadWriteCloser, error) { return nil, errors.New("no such device") })
	if _, err := dnp.Read(ctx, cfg, nil); err == nil {
		t.Error("a port that does not open was used")
	}
	if _, _, err := dnp.OpenSerial(serial.Config{Address: "/dev/x"}); err == nil {
		t.Error("OpenSerial ignored the error of the port")
	}
}
