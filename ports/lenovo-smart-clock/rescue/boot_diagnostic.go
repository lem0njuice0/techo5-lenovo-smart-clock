//go:build !cube_boot

package main

// Diagnostic builds never mount a persistent partition.
func bootIfRequested() error { return nil }
