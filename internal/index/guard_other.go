//go:build !linux && !darwin

package index

// localDiskWarning is a no-op stub so the package builds under non-target GOOS.
func localDiskWarning(string) string { return "" }
