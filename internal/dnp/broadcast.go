package dnp

import (
	"errors"
	"fmt"
	"strings"

	"github.com/otfabric/go-dnp3/link"

	"github.com/otfabric/dnp3ctl/internal/config"
)

// broadcastAddresses maps --broadcast to the broadcast addresses of IEEE 1815-2012 clause
// 9.2.5.3: they differ in whether the outstations ask for an application confirmation.
var broadcastAddresses = map[string]link.Address{
	"no-confirm": link.BroadcastNoConfirm,
	"confirm":    link.BroadcastConfirm,
	"optional":   link.BroadcastOptionalConfirm,
}

// BroadcastModes describes the values of --broadcast.
func BroadcastModes() map[string]string {
	return map[string]string{
		"no-confirm": fmt.Sprintf("address 0x%04X: no application confirmation", uint16(link.BroadcastNoConfirm)),
		"confirm":    fmt.Sprintf("address 0x%04X: confirmation is mandatory", uint16(link.BroadcastConfirm)),
		"optional":   fmt.Sprintf("address 0x%04X: confirmation is optional", uint16(link.BroadcastOptionalConfirm)),
	}
}

// errBroadcastAuth refuses a broadcast with secure authentication. An outstation that runs
// SAv5 discards a critical request to a broadcast address unless it carries an aggressive
// mode MAC, and that MAC covers the last challenge of the connection. Every dnp3ctl command
// opens a connection of its own, so its broadcast is the first request there: nothing has
// been challenged yet, and the outstations would discard it without a word.
var errBroadcastAuth = errors.New("--broadcast cannot be used with secure authentication: an outstation running SAv5 only executes a broadcast with an aggressive mode MAC over an earlier challenge of the same connection, and a dnp3ctl command opens a new connection, so the outstations would discard it")

// checkBroadcast parses --broadcast for the connection cc: empty for a request to the
// outstation, otherwise the name of a broadcast address.
func checkBroadcast(s string, cc *config.ConnConfig) (link.Address, string, error) {
	addr, name, err := parseBroadcast(s)
	if err == nil && name != "" && secureAuthConfigured(cc.AuthConfig) {
		return 0, "", errBroadcastAuth
	}
	return addr, name, err
}

// secureAuthConfigured reports whether a key of secure authentication is given.
func secureAuthConfigured(a config.AuthConfig) bool {
	return a.SAKeyFile != "" || a.SAUpdateKeyFile != "" || strings.TrimSpace(a.SAKey) != "" || strings.TrimSpace(a.SAUpdateKey) != ""
}

// parseBroadcast parses --broadcast: empty for a request to the outstation, otherwise the
// name of a broadcast address.
func parseBroadcast(s string) (link.Address, string, error) {
	name := strings.ToLower(strings.TrimSpace(s))
	if name == "" {
		return 0, "", nil
	}
	addr, ok := broadcastAddresses[name]
	if !ok {
		return 0, "", fmt.Errorf("invalid --broadcast %q: use no-confirm, confirm or optional", s)
	}
	return addr, name, nil
}
