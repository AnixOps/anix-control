package builtin

// The commercial packages. This file is the whole of their link into the
// Control binary: a build without it (the community edition) embeds none of
// them. config/commercial-boundary.json lists it with the rest of the set.

import (
	affiliate "github.com/AnixOps/anix-control/v5/packages/affiliate/control"
	order "github.com/AnixOps/anix-control/v5/packages/order/control"
	payment "github.com/AnixOps/anix-control/v5/packages/payment/control"
)

func init() {
	Register(Host{ID: affiliate.PackageID, Run: affiliate.Run})
	Register(Host{ID: order.PackageID, Run: order.Run})
	Register(Host{ID: payment.PackageID, Run: payment.Run})
}
