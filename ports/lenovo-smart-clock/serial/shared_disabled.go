//go:build !cube_shared

package main

func sharedCommand(string) (string, bool) { return "", false }
func sharedHelp() string                  { return "" }
