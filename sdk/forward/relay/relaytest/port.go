package relaytest

import (
	"errors"
	"math/rand/v2"
	"net"
	"strconv"
	"sync"
)

// Ports a test listens on are drawn below the kernel's ephemeral range
// (Linux: 32768-60999). A port taken with ":0" and released again lies inside
// that range, where any process on the machine may take it as the source port
// of an outbound connection before the test listens: a bind then fails with
// "address already in use" now and then, on a loaded machine.
const (
	firstPort = 20000
	lastPort  = 29999
)

var (
	portMu    sync.Mutex
	portsUsed = map[int]bool{}
)

// FreePort answers a port that is free for TCP and UDP on every address and
// that no earlier call of this process answered.
func FreePort() (uint32, error) {
	portMu.Lock()
	defer portMu.Unlock()
	for range 200 {
		port := firstPort + rand.IntN(lastPort-firstPort+1) // #nosec G404 -- a test port, not a secret
		if portsUsed[port] {
			continue
		}
		l, err := net.Listen("tcp", ":"+strconv.Itoa(port))
		if err != nil {
			continue
		}
		pc, err := net.ListenPacket("udp", ":"+strconv.Itoa(port))
		_ = l.Close()
		if err != nil {
			continue
		}
		_ = pc.Close()
		portsUsed[port] = true
		return uint32(port), nil // #nosec G115 -- a port
	}
	return 0, errors.New("relaytest: no free port")
}
