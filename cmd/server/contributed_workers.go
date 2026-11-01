package main

// contributedWorkers start background workers of packages that are not part
// of the community set; a file that registers one (order_payments.go) can be
// left out of a build.
var contributedWorkers []func(rt *serverRuntime)

//nolint:unused // called by files only the commercial build adds (order_payments.go)
func registerContributedWorker(start func(rt *serverRuntime)) {
	contributedWorkers = append(contributedWorkers, start)
}

func (rt *serverRuntime) startContributedWorkers() {
	for _, start := range contributedWorkers {
		start(rt)
	}
}
