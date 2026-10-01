// Package kernelnodeopsv1 holds the KernelNodeOps contract
// (anixops.kernelnodeops.v1): typed, idempotent node operations that official
// packages ask the kernel to carry out, designed in
// docs/architecture/node-ops-service.md. The kernel serves it on the local
// package bridge and the module listener; it is binding and grows by
// additions only. GetCapabilities answers which operation kinds a kernel
// executes.
package kernelnodeopsv1
