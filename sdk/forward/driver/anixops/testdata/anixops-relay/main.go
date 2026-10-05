// Command anixops-relay is the relay program the network namespace tests run
// (netns_test.go): the same main the Agent ships, as a package the SDK can
// build without the Agent.
package main

import (
	"os"

	"github.com/AnixOps/anix-control/sdk/forward/driver/anixops/relayd"
)

func main() { os.Exit(relayd.Main(os.Args[1:], os.Stdout, os.Stderr)) }
