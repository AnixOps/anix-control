package anixops

// SetSysctlDir points Probe at a directory standing in for /proc/sys/net/core.
func SetSysctlDir(dir string) (restore func()) {
	old := sysctlDir
	sysctlDir = dir
	return func() { sysctlDir = old }
}
