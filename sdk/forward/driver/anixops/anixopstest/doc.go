// Package anixopstest runs the anixops relay inside the test process, so the
// driver (sdk/forward/driver/anixops) is tested against a real relay, real
// sockets and real files without privileges, a network namespace or systemd:
// Host is a relay supervisor that starts and stops a relayd.Relay on a
// control socket in a temporary directory, shares it between driver
// instances (an Agent restart is a new driver on the same host) and can crash
// it, fail its next apply or redirect its dials to a local target.
package anixopstest
