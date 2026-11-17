//go:build !cube_update

package main

import "os"

func updateCommand(string) (string, bool) { return "", false }
func prepareSerial(*os.File) error        { return nil }
