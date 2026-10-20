package dnp

import (
	"fmt"
	"strings"

	"github.com/otfabric/go-dnp3/link"
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
		"no-confirm": fmt.Sprintf("address %#04X: no application confirmation", uint16(link.BroadcastNoConfirm)),
		"confirm":    fmt.Sprintf("address %#04X: the outstations ask for confirmation", uint16(link.BroadcastConfirm)),
		"optional":   fmt.Sprintf("address %#04X: confirmation is optional", uint16(link.BroadcastOptionalConfirm)),
	}
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
