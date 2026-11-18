//go:build cube && !dot && !spot && !panel

package firmware

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"syscall"

	"github.com/HuskerMinion/techo5/echod/internal/feature/security"
	"github.com/HuskerMinion/techo5/echod/internal/lib/safe"
	esphome "github.com/ygelfand/go-esphome-device"
)

var cubeUpdateBusy atomic.Bool
var cubeUpdateEncrypted = security.Encrypted
var cubeUpdateProvisioned = cubeUpdateAvailable

// cubeUpdateCommand has no user-controlled arguments. The root-owned helper
// reads the pinned public key and the separately provisioned HTTPS source.
var cubeUpdateCommand = func() error {
	cmd := exec.Command("/usr/local/sbin/cube-serial", "ota-update")
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
	out, err := cmd.CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(out))
		if len(text) > 512 {
			text = text[len(text)-512:]
		}
		return fmt.Errorf("cube updater: %w: %s", err, text)
	}
	return nil
}

func cubeUpdateAvailable() bool {
	marker, err := os.ReadFile("/etc/techo5-cube-root")
	if err != nil || string(marker) != "TECHO5-CUBE-ROOT-V1\n" {
		return false
	}
	info, err := os.Stat("/usr/local/sbin/cube-serial")
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
		return false
	}
	key, err := os.ReadFile("/etc/techo5-cube/update-signing.pub")
	return err == nil && len(key) == 32
}

// Actions exposes an explicit update action only on a provisioned cube. The
// Home Assistant link must be encrypted; the action cannot select a URL or key.
func (u *Firmware) Actions() []*esphome.Action {
	if !cubeUpdateProvisioned() {
		return nil
	}
	return []*esphome.Action{{Name: "cube_update_now", Run: func(esphome.Call) (any, error) {
		if !cubeUpdateEncrypted() {
			return nil, errors.New("cube update requires an encrypted Home Assistant connection")
		}
		if !cubeUpdateBusy.CompareAndSwap(false, true) {
			return nil, errors.New("cube update is already running")
		}
		u.busy.Add(1)
		safe.Go("cube update", func() {
			defer cubeUpdateBusy.Store(false)
			defer u.busy.Add(-1)
			if u.status != nil {
				u.status.Set("Checking and installing signed update")
			}
			if err := cubeUpdateCommand(); err != nil {
				slog.Warn("cube update failed", "error", err)
				if u.status != nil {
					u.status.Set("Update failed; check the configured source and device logs")
				}
				return
			}
			if u.status != nil {
				u.status.Set("Update staged")
			}
		})
		return nil, nil
	}}}
}
