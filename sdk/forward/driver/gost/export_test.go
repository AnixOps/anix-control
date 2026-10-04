package gost

// SetAPIFault makes d refuse a changing web API request whenever f answers
// an error for it, as if gost had refused it: the netns conformance Env's
// ApplyFaulter, for applies that change gost through the web API.
func SetAPIFault(d *Driver, f func(method, path string) error) { d.apiFault = f }
