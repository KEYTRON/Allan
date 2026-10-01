package link

import (
	"crypto/rand"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
)

// DefaultWorkerPort is where `allan serve` listens by default.
const DefaultWorkerPort = "8790"

// MachineID returns a stable identifier of this computer. It is created on the
// first call and kept in <config dir>/machine_id, so it survives restarts,
// upgrades and re-pairing: the site uses it to tell "the same PC connected
// again" from "another PC", and the user sees it in `allan id` and `--help`.
func MachineID() string {
	dir := configDir()
	path := filepath.Join(dir, "machine_id")
	if raw, err := os.ReadFile(path); err == nil {
		if id := strings.TrimSpace(string(raw)); validMachineID(id) {
			return id
		}
	}
	id := newUUID()
	if err := os.MkdirAll(dir, 0o700); err == nil {
		_ = os.WriteFile(path, []byte(id+"\n"), 0o600)
	}
	return id
}

// ShortID is the first block of the id: enough to tell machines apart by eye.
func ShortID(id string) string {
	if i := strings.IndexByte(id, '-'); i > 0 {
		return id[:i]
	}
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// MachineName is how this computer is labelled by default: its hostname without
// the domain part (gentoo-pc, not gentoo-pc.tail1234.ts.net).
func MachineName() string {
	host, err := os.Hostname()
	if err != nil || strings.TrimSpace(host) == "" {
		return "Рабочая машина"
	}
	if i := strings.IndexByte(host, '.'); i > 0 {
		host = host[:i]
	}
	return host
}

// TailscaleAddress returns the URL of the worker on this machine's Tailscale
// address (100.64.0.0/10), or "" when the machine is not on a tailnet. The site
// reaches the worker over Tailscale, so this is the address it needs.
func TailscaleAddress(port string) string {
	if port == "" {
		port = DefaultWorkerPort
	}
	_, cgnat, _ := net.ParseCIDR("100.64.0.0/10")
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		if ip4 := ipnet.IP.To4(); ip4 != nil && cgnat.Contains(ip4) {
			return "http://" + net.JoinHostPort(ip4.String(), port)
		}
	}
	return ""
}

func validMachineID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for i, c := range id {
		switch {
		case i == 8 || i == 13 || i == 18 || i == 23:
			if c != '-' {
				return false
			}
		case !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f'):
			return false
		}
	}
	return true
}

// newUUID makes a random (version 4) UUID without extra dependencies.
func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
