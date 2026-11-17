// cube-rescue is an experimental, read-only diagnostic init for CD-24501F.
// The diagnostic variant never mounts persistent block devices. The separately
// built cube_boot variant can mount an explicitly requested, marked root.
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func log(s string, args ...any) {
	msg := fmt.Sprintf("cube-rescue: "+s+"\n", args...)
	if f, err := os.OpenFile("/dev/kmsg", os.O_WRONLY, 0); err == nil {
		_, _ = f.WriteString(msg)
		_ = f.Close()
	}
	_, _ = io.WriteString(os.Stderr, msg)
}

func mount(kind, target string) error {
	if err := os.MkdirAll(target, 0755); err != nil {
		return err
	}
	if err := syscall.Mount(kind, target, kind, 0, ""); err != nil && err != syscall.EBUSY {
		return err
	}
	return nil
}

func write(path, value string) error { return os.WriteFile(path, []byte(value), 0644) }

var bootStatus = "root startup was not attempted"

// gadget creates only a USB serial device; it exposes neither network nor disk.
func gadget(root, udcRoot, switchPath string) error {
	if err := write(switchPath, "device"); err != nil {
		return fmt.Errorf("USB mode: %w", err)
	}
	time.Sleep(time.Second)
	g := filepath.Join(root, "cube-rescue")
	dirs := []string{g, filepath.Join(g, "strings/0x409"), filepath.Join(g, "configs/c.1/strings/0x409"), filepath.Join(g, "functions/acm.usb0")}
	for _, p := range dirs {
		if err := os.MkdirAll(p, 0755); err != nil {
			return err
		}
	}
	fields := [][2]string{{"idVendor", "0x1d6b"}, {"idProduct", "0x0104"}, {"bcdUSB", "0x0200"}, {"strings/0x409/manufacturer", "TECHO5"}, {"strings/0x409/product", "Smart Clock diagnostic"}, {"strings/0x409/serialnumber", "cube-rescue"}, {"configs/c.1/strings/0x409/configuration", "serial"}, {"configs/c.1/MaxPower", "250"}}
	for _, f := range fields {
		if err := write(filepath.Join(g, f[0]), f[1]); err != nil {
			return fmt.Errorf("%s: %w", f[0], err)
		}
	}
	if err := os.Symlink(filepath.Join(g, "functions/acm.usb0"), filepath.Join(g, "configs/c.1/acm.usb0")); err != nil && !os.IsExist(err) {
		return err
	}
	entries, err := os.ReadDir(udcRoot)
	if err != nil {
		return err
	}
	if len(entries) != 1 {
		return fmt.Errorf("expected one USB controller, found %d", len(entries))
	}
	return write(filepath.Join(g, "UDC"), entries[0].Name())
}

func command(s string, read func(string) ([]byte, error)) string {
	switch strings.TrimSpace(s) {
	case "help":
		return "commands: help, version, cmdline, partitions, input, sound, pcm, framebuffer, status\n"
	case "version", "cmdline", "partitions", "input", "sound", "pcm", "framebuffer":
		name := map[string]string{"version": "/proc/version", "cmdline": "/proc/cmdline", "partitions": "/proc/partitions", "input": "/proc/bus/input/devices", "sound": "/proc/asound/cards", "pcm": "/proc/asound/pcm", "framebuffer": "/proc/fb"}[strings.TrimSpace(s)]
		b, err := read(name)
		if err != nil {
			return fmt.Sprintf("unavailable: %v\n", err)
		}
		return string(b) + "\n"
	case "status":
		return "diagnostic mode; " + bootStatus + "\n"
	case "":
		return ""
	default:
		if out, ok := updateCommand(strings.TrimSpace(s)); ok {
			return out
		}
		if out, ok := slotCommand(strings.TrimSpace(s)); ok {
			return out
		}
		return "unknown command; type help\n"
	}
}

func serve(rw io.ReadWriter) {
	_, _ = io.WriteString(rw, "TECHO5 cube diagnostic (experimental)\ntype help\n> ")
	scanner := bufio.NewScanner(rw)
	for scanner.Scan() {
		_, _ = io.WriteString(rw, command(scanner.Text(), os.ReadFile)+"> ")
	}
}

func main() {
	for _, m := range [][2]string{{"proc", "/proc"}, {"sysfs", "/sys"}, {"devtmpfs", "/dev"}, {"configfs", "/sys/kernel/config"}} {
		if err := mount(m[0], m[1]); err != nil {
			log("mount %s: %v", m[1], err)
		}
	}
	if err := gadget("/sys/kernel/config/usb_gadget", "/sys/class/udc", "/sys/devices/platform/soc/mt_usb/swmode"); err != nil {
		log("USB setup failed: %v", err)
	} else {
		log("USB serial gadget configured")
	}
	// Only a separately built cube_boot variant with an explicit kernel argument
	// may try the marked system_b root. The normal diagnostic never mounts it.
	if err := bootIfRequested(); err != nil {
		log("root startup: %v", err)
		bootStatus = "root startup failed: " + err.Error()
	} else {
		bootStatus = "no marked root started"
	}
	// Keep PID 1 alive even when the controller is unavailable or the host disconnects.
	for {
		f, err := os.OpenFile("/dev/ttyGS0", os.O_RDWR, 0)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		if err := prepareSerial(f); err != nil {
			log("serial setup: %v", err)
			_ = f.Close()
			time.Sleep(time.Second)
			continue
		}
		serve(f)
		_ = f.Close()
		time.Sleep(time.Second)
	}
}
