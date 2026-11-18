//go:build cube_slot && cube_update_trial

package main

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
)

// openCubeIncoming opens only files in a caller-validated, root-owned private
// directory. It verifies that each file belongs to the shared data filesystem.
func openCubeIncoming(path string, max int64, dataDevice uint64) (*os.File, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 || st.Uid != 0 || st.Nlink != 1 || uint64(st.Dev) != dataDevice || info.Size() <= 0 || info.Size() > max {
		f.Close()
		return nil, errors.New("unsafe incoming update file")
	}
	return f, nil
}

func runOptionalUpdateCommand(args []string) error {
	if len(args) != 1 || (args[0] != "ota-fetch" && args[0] != "ota-stage" && args[0] != "ota-monitor" && args[0] != "ota-watch") {
		return errors.New("unknown experimental update command")
	}
	if os.Geteuid() != 0 {
		return errors.New("update command requires root")
	}
	key, err := readCubeMarker("/etc/techo5-cube/update-signing.pub")
	if err != nil {
		return err
	}
	if len(key) != ed25519.PublicKeySize {
		return errors.New("invalid pinned update key")
	}
	if args[0] == "ota-monitor" {
		return monitorCubeTrial(context.Background(), ed25519.PublicKey(key))
	}
	if args[0] == "ota-watch" {
		return watchCubeUpdate(context.Background(), ed25519.PublicKey(key))
	}
	if args[0] == "ota-fetch" {
		return fetchConfiguredCubeUpdate(context.Background(), ed25519.PublicKey(key))
	}
	if err := secureCubeDirectory("/data/techo5-linux/incoming"); err != nil {
		return err
	}
	dataInfo, err := os.Stat("/data")
	if err != nil {
		return err
	}
	dataStat, ok := dataInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.New("shared data device unavailable")
	}
	base := "/data/techo5-linux/incoming/"
	manifest, err := openCubeIncoming(base+"manifest.json", 16384, uint64(dataStat.Dev))
	if err != nil {
		return err
	}
	defer manifest.Close()
	sig, err := openCubeIncoming(base+"manifest.sig", ed25519.SignatureSize, uint64(dataStat.Dev))
	if err != nil {
		return err
	}
	defer sig.Close()
	system, err := openCubeIncoming(base+"system.img", 1024*1024*1024, uint64(dataStat.Dev))
	if err != nil {
		return err
	}
	defer system.Close()
	boot, err := openCubeIncoming(base+"boot.img", 64*1024*1024, uint64(dataStat.Dev))
	if err != nil {
		return err
	}
	defer boot.Close()
	payload, err := io.ReadAll(io.LimitReader(manifest, 16385))
	if err != nil {
		return err
	}
	signature, err := io.ReadAll(io.LimitReader(sig, ed25519.SignatureSize+1))
	if err != nil {
		return err
	}
	if err := stageCubeUpdate(ed25519.PublicKey(key), payload, signature, system, boot); err != nil {
		return err
	}
	fmt.Println("Update staged as a trial; a supervised reboot and health monitor are required.")
	return nil
}

// fetchConfiguredCubeUpdate uses a root-controlled HTTPS source. The source
// can be private; release firmware is not assumed to be publicly distributable.
func fetchConfiguredCubeUpdate(ctx context.Context, key ed25519.PublicKey) error {
	if err := secureCubeDirectory("/run/techo5"); err != nil {
		return err
	}
	lock, err := acquireCubeUpdateLock("/run/techo5/cube-update.lock")
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := secureCubeDirectory("/data/techo5-linux"); err != nil {
		return err
	}
	source, err := readCubeRootFile("/data/techo5-linux/cube-update-source", 2048)
	if err != nil {
		return err
	}
	base := strings.TrimSuffix(string(source), "\n")
	if string(source) != base+"\n" {
		return errors.New("invalid update source configuration")
	}
	if _, err := cubeUpdateURL(base, "manifest-a.json"); err != nil {
		return err
	}
	ca, err := readCubeRootFile("/data/techo5-linux/cube-update-ca.pem", 16384)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if os.IsNotExist(err) {
		ca = nil
	}
	client, err := cubeUpdateClient(ca)
	if err != nil {
		return err
	}
	cmd, err := os.ReadFile("/proc/cmdline")
	if err != nil {
		return err
	}
	current := ""
	count := 0
	for _, word := range strings.Fields(string(cmd)) {
		if strings.HasPrefix(word, "androidboot.slot_suffix=") {
			count++
			if word == "androidboot.slot_suffix=_a" {
				current = "a"
			} else if word == "androidboot.slot_suffix=_b" {
				current = "b"
			} else {
				return errors.New("invalid running slot")
			}
		}
	}
	if count != 1 {
		return errors.New("ambiguous running slot")
	}
	target := "a"
	if current == "a" {
		target = "b"
	}
	system, err := discoverCubePartition("/sys/class/block", "/dev", "system_"+target)
	if err != nil {
		return err
	}
	_, nodes, err := inspectCubeUpdateDevice(cubeUpdateManifest{Slot: target, SystemSize: system.size, BootSize: 1})
	if err != nil {
		return err
	}
	if err := secureCubeDirectory("/data/techo5-linux"); err != nil {
		return err
	}
	floor, err := readCubeSequence("/data/techo5-linux/cube-update-sequence")
	if err != nil {
		return err
	}
	if _, err := os.Lstat("/data/techo5-linux/cube-update-pending.json"); err == nil {
		return errors.New("pending update requires resolution")
	} else if !os.IsNotExist(err) {
		return err
	}
	bootSize := int64(0)
	for _, node := range nodes {
		if node.name == "boot_"+target {
			bootSize = node.size
		}
	}
	if err := fetchCubeBundle(ctx, client, base, "/data/techo5-linux/incoming", key, target, floor, system.size, bootSize); err != nil {
		return err
	}
	fmt.Println("Signed update downloaded and verified; staging has not started.")
	return nil
}
