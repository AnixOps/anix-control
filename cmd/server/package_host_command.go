package main

import (
	"context"
	"os"

	"github.com/AnixOps/anix-control/v5/internal/builtinpkg"
	"github.com/AnixOps/anix-control/v5/packages/builtin"
)

// runPackageHostCommand runs `anix-control package-host <id>`: an official
// package host linked into this binary (packages/builtin). The kernel starts
// it by executing its own binary (/proc/self/exe) with the package host
// environment of internal/pluginhost, and the version it verified in
// ANIX_CONTROL_HOST_PACKAGE_VERSION. It reads no configuration file and opens
// no database: the host reaches the kernel only through its package bridge.
// With ANIX_MODULE_MODE=remote it runs as a network module instead.
func runPackageHostCommand(args []string) (int, bool) {
	if len(args) == 0 || args[0] != builtin.CommandName {
		return 0, false
	}
	return builtin.Main(context.Background(), args[1:], builtin.Options{
		Getenv: os.Getenv, Version: builtinpkg.HostVersion, Stderr: os.Stderr,
	}), true
}
