//go:build !cube_slot

package main

func slotCommand(string) (string, bool) { return "", false }
func slotHelp() string                  { return "" }
