// Package networkv1 holds the device network contract (anixops.network.v1):
// the NetworkPlan Control renders for each managed device from the model of
// lines, ingresses, egresses, transports and paths, the DeviceCapabilities
// an Agent offers in its Hello, and the NetworkStatus it reports. It is
// designed in docs/design/network-model; sdk/network carries it over the
// Agent Control stream and checks it. It is a draft until Control serves it.
package networkv1
