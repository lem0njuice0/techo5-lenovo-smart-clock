//go:build cube && !dot && !spot && !panel

package firmware

import (
	"testing"
	"time"

	esphome "github.com/ygelfand/go-esphome-device"
)

func TestCubeUpdateActionRequiresProvisioningEncryptionAndSingleRun(t *testing.T) {
	oldProvisioned, oldEncrypted, oldCommand := cubeUpdateProvisioned, cubeUpdateEncrypted, cubeUpdateCommand
	t.Cleanup(func() {
		cubeUpdateProvisioned, cubeUpdateEncrypted, cubeUpdateCommand = oldProvisioned, oldEncrypted, oldCommand
		cubeUpdateBusy.Store(false)
	})
	u := &Firmware{}
	cubeUpdateProvisioned = func() bool { return false }
	if len(u.Actions()) != 0 {
		t.Fatal("unprovisioned update action was exposed")
	}
	cubeUpdateProvisioned = func() bool { return true }
	cubeUpdateEncrypted = func() bool { return false }
	actions := u.Actions()
	if len(actions) != 1 || actions[0].Name != "cube_update_now" || len(actions[0].Args) != 0 {
		t.Fatal("unexpected action")
	}
	if _, err := actions[0].Run(esphome.Call{}); err == nil {
		t.Fatal("accepted an unencrypted update")
	}
	started, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	cubeUpdateEncrypted = func() bool { return true }
	cubeUpdateCommand = func() error { close(started); <-release; close(finished); return nil }
	if _, err := actions[0].Run(esphome.Call{}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("update did not start")
	}
	if _, err := actions[0].Run(esphome.Call{}); err == nil {
		t.Fatal("accepted a concurrent update")
	}
	close(release)
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("update did not finish")
	}
	deadline := time.Now().Add(time.Second)
	for cubeUpdateBusy.Load() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if cubeUpdateBusy.Load() || u.busy.Load() != 0 {
		t.Fatal("update remained busy")
	}
}
