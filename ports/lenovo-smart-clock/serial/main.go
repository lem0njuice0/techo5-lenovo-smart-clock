// cube-serial is a limited diagnostic on the initramfs USB gadget.
// It never configures USB, runs arbitrary commands, or opens a shell. The
// optional cube_slot build can mark the current trial slot successful after
// checking that the other slot remains successful. The optional cube_shared
// build can disable shared-data startup after a detected migration failure.
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

var nodes = []string{"/dev/fb0", "/dev/input", "/dev/snd", "/sys/class/net/wlan0", "/sys/class/leds/lcd-backlight", "/dev/i2c-0"}

func command(line string, stat func(string) (os.FileInfo, error), read func(string) ([]byte, error)) string {
	switch strings.TrimSpace(line) {
	case "help":
		return "commands: help, status, nodes, version, cmdline, audio-status" + slotHelp() + sharedHelp() + "\n"
	case "status":
		if b, err := read("/etc/techo5-cube-root"); err == nil && string(b) == "TECHO5-CUBE-ROOT-V1\n" {
			return "experimental cube root active; hardware validation required\n"
		}
		return "cube root marker unavailable\n"
	case "nodes":
		var out strings.Builder
		for _, p := range nodes {
			_, err := stat(p)
			fmt.Fprintf(&out, "%s: %s\n", p, map[bool]string{true: "present", false: "unavailable"}[err == nil])
		}
		return out.String()
	case "version", "cmdline":
		path := map[string]string{"version": "/proc/version", "cmdline": "/proc/cmdline"}[strings.TrimSpace(line)]
		b, err := read(path)
		if err != nil {
			return "unavailable\n"
		}
		return strings.TrimSpace(string(b)) + "\n"
	case "audio-status":
		return audioStatus()
	case "":
		return ""
	default:
		if out, ok := sharedCommand(strings.TrimSpace(line)); ok {
			return out
		}
		if out, ok := slotCommand(strings.TrimSpace(line)); ok {
			return out
		}
		return "unknown command; type help\n"
	}
}

// Only this fixed log is exposed, with a bounded read. No arbitrary paths or commands.
func audioStatus() string {
	f, err := os.Open("/var/log/cube-audio.log")
	if err != nil {
		return "audio log unavailable\n"
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "audio log unavailable\n"
	}
	if info.Size() > 4096 {
		_, _ = f.Seek(-4096, io.SeekEnd)
	}
	b, err := io.ReadAll(io.LimitReader(f, 4096))
	if err != nil {
		return "audio log unavailable\n"
	}
	if len(b) == 0 {
		return "audio log is empty\n"
	}
	return string(b) + "\n"
}

func serve(rw io.ReadWriter) {
	fmt.Fprint(rw, "TECHO5 cube runtime diagnostic (experimental)\ntype help\n> ")
	scanner := bufio.NewScanner(rw)
	for scanner.Scan() {
		fmt.Fprint(rw, command(scanner.Text(), os.Stat, os.ReadFile), "> ")
	}
}

func main() {
	if len(os.Args) > 1 {
		if err := runOptionalUpdateCommand(os.Args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	for {
		f, err := os.OpenFile("/dev/ttyGS0", os.O_RDWR, 0)
		if err == nil {
			serve(f)
			f.Close()
		}
		time.Sleep(time.Second)
	}
}
