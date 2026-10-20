// Package stationtest starts the simulated outstation of package station for tests, so
// that nothing in the test suite needs a real device.
package stationtest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/otfabric/dnp3ctl/internal/station"
	"github.com/otfabric/dnp3ctl/internal/types"
)

// Model is the model the tests run against: a point of every type, controls, files and
// uploads. Nothing in it changes by itself, so that results are exact.
const Model = `
outstation:
  name: Test outstation
  address: 10
  unsolicited: {enabled: true}
  restart_delay: 1s
  attributes: {manufacturer: otfabric, product: test outstation, serial-number: SN-1}
points:
  - {type: binary-input, index: 0, value: true, class: 1}
  - {type: binary-input, index: 1, value: false, class: 1, flags: [ONLINE, CHATTER_FILTER]}
  - {type: double-bit-input, index: 0, value: "on", class: 1}
  - {type: binary-output, index: 0, value: true, class: 1}
  - {type: binary-output, index: 1, value: false}
  - {type: counter, index: 0, value: 123456, class: 3}
  - {type: frozen-counter, index: 0, value: 0}
  - {type: analog-input, index: 0, value: 230.5, class: 2, static_variation: 5}
  - {type: analog-input, index: 1, value: 42, class: 2, flags: [ONLINE, OVER_RANGE]}
  - {type: frozen-analog-input, index: 0, value: 0}
  - {type: analog-output, index: 0, value: 11, static_variation: 3}
  - {type: octet-string, index: 0, value: hello}
controls:
  - {type: crob, index: 0, target: {type: binary-output, index: 0}, select_required: true}
  - {type: crob, index: 1}
  - {type: crob, index: 2, status: LOCAL}
  - {type: crob, index: 3, target: {type: binary-input, index: 1}}
  - {type: analog, index: 0, min: 0, max: 100}
files:
  - {name: /readme.txt, content: "hello from the outstation\n"}
  - {name: /records/big.dat, size: 5000}
uploads: {accept: true}
`

// Station is an outstation serving on a loopback port.
type Station struct {
	*station.Station
	// Host and Port are where the outstation listens.
	Host string
	Port uint16

	mu     sync.Mutex
	events []types.ServerEvent
}

// Addr returns host:port.
func (s *Station) Addr() string { return net.JoinHostPort(s.Host, strconv.Itoa(int(s.Port))) }

// Events returns the events the outstation has logged so far.
func (s *Station) Events() []types.ServerEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]types.ServerEvent(nil), s.events...)
}

// HasEvent reports whether an event of the given kind was logged.
func (s *Station) HasEvent(kind string) bool {
	for _, e := range s.Events() {
		if e.Event == kind {
			return true
		}
	}
	return false
}

// Start serves the model on a loopback port until the test ends. edit, when given, changes
// the options before the outstation is built.
func Start(t testing.TB, model string, edit ...func(*station.Options)) *Station {
	t.Helper()
	m, err := station.Parse([]byte(model))
	if err != nil {
		t.Fatalf("stationtest: model: %v", err)
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("stationtest: model: %v", err)
	}
	out := &Station{Host: "127.0.0.1"}
	opts := station.Options{Log: func(e types.ServerEvent) {
		out.mu.Lock()
		out.events = append(out.events, e)
		out.mu.Unlock()
	}}
	for _, f := range edit {
		f(&opts)
	}
	st, err := station.New(m, opts)
	if err != nil {
		t.Fatalf("stationtest: %v", err)
	}
	var ln net.Listener
	ln, err = net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("stationtest: listen: %v", err)
	}
	if opts.TLS != nil {
		ln = tls.NewListener(ln, opts.TLS)
	}
	out.Station = st
	out.Port = uint16(ln.Addr().(*net.TCPAddr).Port)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = st.Serve(ln)
	}()
	t.Cleanup(func() {
		_ = st.Close()
		<-done
	})
	return out
}

// FreePort returns a loopback TCP port that was free a moment ago.
func FreePort(t testing.TB) uint16 {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	return uint16(ln.Addr().(*net.TCPAddr).Port)
}

// Certs is a throw-away certificate authority with a server and a client certificate, as
// PEM files in a temporary directory.
type Certs struct {
	CA, ServerCert, ServerKey, ClientCert, ClientKey string
	// ServerTLS is a server configuration that requires a client certificate of the CA.
	ServerTLS *tls.Config
}

// NewCerts creates the certificates. The server certificate is valid for 127.0.0.1 and
// localhost.
func NewCerts(t testing.TB) Certs {
	t.Helper()
	dir := t.TempDir()
	write := func(name, kind string, der []byte) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: der}), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	key := func() *ecdsa.PrivateKey {
		k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		return k
	}
	template := func(serial int64, name string) *x509.Certificate {
		return &x509.Certificate{
			SerialNumber: big.NewInt(serial),
			Subject:      pkix.Name{CommonName: name},
			NotBefore:    time.Now().Add(-time.Hour),
			NotAfter:     time.Now().Add(time.Hour),
		}
	}
	caKey := key()
	caTmpl := template(1, "dnp3ctl test CA")
	caTmpl.IsCA, caTmpl.BasicConstraintsValid, caTmpl.KeyUsage = true, true, x509.KeyUsageCertSign
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caCert, _ := x509.ParseCertificate(caDER)
	leaf := func(serial int64, name string, usage x509.ExtKeyUsage) (certPath, keyPath string) {
		k := key()
		tmpl := template(serial, name)
		tmpl.KeyUsage = x509.KeyUsageDigitalSignature
		tmpl.ExtKeyUsage = []x509.ExtKeyUsage{usage}
		tmpl.DNSNames = []string{"localhost"}
		tmpl.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
		der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &k.PublicKey, caKey)
		if err != nil {
			t.Fatal(err)
		}
		keyDER, err := x509.MarshalECPrivateKey(k)
		if err != nil {
			t.Fatal(err)
		}
		return write(name+".pem", "CERTIFICATE", der), write(name+".key", "EC PRIVATE KEY", keyDER)
	}
	c := Certs{CA: write("ca.pem", "CERTIFICATE", caDER)}
	c.ServerCert, c.ServerKey = leaf(2, "server", x509.ExtKeyUsageServerAuth)
	c.ClientCert, c.ClientKey = leaf(3, "client", x509.ExtKeyUsageClientAuth)

	pair, err := tls.LoadX509KeyPair(c.ServerCert, c.ServerKey)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(caCert)
	c.ServerTLS = &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{pair}, ClientCAs: pool, ClientAuth: tls.RequireAndVerifyClientCert}
	return c
}
