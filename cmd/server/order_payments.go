package main

import (
	"context"
	"log"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/service"
)

// orderPaymentReconcile is how often the kernel completes the orders that
// paid payments left pending.
const orderPaymentReconcile = 5 * time.Minute

// startOrderPaymentReconciler completes the order of a paid payment record
// whose callback did not (docs/architecture/order-service.md): the payment
// module commits the record before it asks KernelOrder, and a provider that
// never delivers again leaves the order pending. Every Control process runs
// it; the record's row lock and its request id apply a payment once.
func (rt *serverRuntime) startOrderPaymentReconciler() {
	reconciler := service.NewOrderPaymentReconciler(database.Get())
	rt.workers.Go("order payment reconciler", func(ctx context.Context) {
		for {
			if _, err := reconciler.RunOnce(ctx, time.Now()); err != nil && ctx.Err() == nil {
				log.Printf("Order payment reconciliation failed: %v", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(orderPaymentReconcile):
			}
		}
	})
}
