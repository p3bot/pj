//go:build !linux && !darwin

package cli

// supportedOS is false outside macOS/Linux (fail closed, no half-run).
func supportedOS() bool { return false }
