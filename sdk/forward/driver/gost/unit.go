package gost

import (
	"fmt"
	"regexp"
	"strings"
)

// Unit is what UnitFile needs: where the Agent's package put gost, the
// driver's directories, and the account gost runs as.
type Unit struct {
	// Binary is the pinned gost the Agent ships (DefaultBinary).
	Binary string
	// Dir and RuntimeDir are the driver's Config.Dir and Config.RuntimeDir.
	// RuntimeDir must be /run/<name>: the unit creates it.
	Dir        string
	RuntimeDir string
	// User and Group are gost's own account (DefaultUser): not the
	// Agent's, and without a shell or home.
	User  string
	Group string
}

// DefaultUser is the account gost runs as.
const DefaultUser = "anixops-gost"

// DefaultUnit answers the unit for the default paths and account.
func DefaultUnit() Unit {
	return Unit{Binary: DefaultBinary, Dir: DefaultDir, RuntimeDir: DefaultRuntimeDir, User: DefaultUser, Group: DefaultUser}
}

var accountName = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

// UnitFile renders anixops-gost.service (owner decisions H13 and H20).
// The installer writes it to /etc/systemd/system and enables it; the
// Agent's driver only starts, reloads and stops it (a polkit
// rule the installer adds), and Remove leaves it installed. Uninstall
// removes it.
//
// gost runs as its own user with CAP_NET_BIND_SERVICE only: it forwards
// sockets and needs no CAP_NET_ADMIN (that is the Agent's, for nftables
// and tc). Everything but its runtime directory is read only to it, so a
// compromised gost can neither rewrite its configuration nor reach the
// Agent's keys. The Agent belongs to the unit's group so it can read the
// metrics socket; Dir is owned by the Agent with that group, mode 0750.
func UnitFile(u Unit) ([]byte, error) {
	for _, f := range []struct{ what, p string }{{"Binary", u.Binary}, {"Dir", u.Dir}, {"RuntimeDir", u.RuntimeDir}} {
		if err := checkPath(f.what, f.p, false); err != nil {
			return nil, err
		}
	}
	runtimeName, ok := strings.CutPrefix(u.RuntimeDir, "/run/")
	if !ok || runtimeName == "" || strings.Contains(runtimeName, "/") {
		return nil, fmt.Errorf("%w: RuntimeDir %q is not /run/<name>", ErrInvalidConfig, u.RuntimeDir)
	}
	if !accountName.MatchString(u.User) || !accountName.MatchString(u.Group) {
		return nil, fmt.Errorf("%w: user or group is not an account name", ErrInvalidConfig)
	}
	config := u.Dir + "/" + ConfigFile
	return []byte(`# ` + UnitName + `: the gost of the AnixOps Agent's forward driver.
# Rendered by sdk/forward/driver/gost (UnitFile); installed by the AnixOps
# installer. The Agent starts, reloads (SIGHUP) and stops it; it is a unit
# of its own so that restarting or upgrading the Agent keeps forwarding.
[Unit]
Description=AnixOps forward gost (` + OwnerMark + `)
Documentation=https://github.com/AnixOps/anix-control/blob/go_dev/docs/architecture/forward-sdk.md
After=network-online.target
Wants=network-online.target
# Without a configuration there is nothing to serve: the driver writes it
# on its first apply and deletes it on Remove.
ConditionPathExists=` + config + `

[Service]
Type=exec
ExecStart=` + u.Binary + ` -C ` + config + `
ExecReload=/bin/kill -HUP $MAINPID
Restart=on-failure
RestartSec=2s
TimeoutStopSec=10s
User=` + u.User + `
Group=` + u.Group + `
UMask=0007
RuntimeDirectory=` + runtimeName + `
RuntimeDirectoryMode=0750
LimitNOFILE=1048576

# Privileges: binding ports below 1024, nothing else.
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
NoNewPrivileges=yes

# Sandbox: the file system is read only but for RuntimeDirectory.
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes
PrivateDevices=yes
ProtectKernelTunables=yes
ProtectKernelModules=yes
ProtectKernelLogs=yes
ProtectControlGroups=yes
ProtectClock=yes
ProtectHostname=yes
ProtectProc=invisible
ProcSubset=pid
RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX
RestrictNamespaces=yes
RestrictRealtime=yes
RestrictSUIDSGID=yes
LockPersonality=yes
MemoryDenyWriteExecute=yes
RemoveIPC=yes
SystemCallArchitectures=native
SystemCallFilter=@system-service
SystemCallFilter=~@privileged
SystemCallErrorNumber=EPERM

[Install]
WantedBy=multi-user.target
`), nil
}
