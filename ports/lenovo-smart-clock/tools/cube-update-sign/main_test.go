package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/binary"
	"encoding/json"
	"encoding/pem"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSignPrivateBundle(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	d := t.TempDir()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	key := filepath.Join(d, "private.pem")
	if err := os.WriteFile(key, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0600); err != nil {
		t.Fatal(err)
	}
	system := make([]byte, 2048)
	system[1080], system[1081] = 0x53, 0xef
	boot := make([]byte, 6144)
	copy(boot, "ANDROID!")
	binary.LittleEndian.PutUint32(boot[8:], 1)
	binary.LittleEndian.PutUint32(boot[16:], 1)
	binary.LittleEndian.PutUint32(boot[36:], 2048)
	copy(boot[64:], "keep_initramfs techo5.cube.root=system_a")
	sysPath, bootPath := filepath.Join(d, "system.img"), filepath.Join(d, "boot.img")
	if err := os.WriteFile(sysPath, system, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bootPath, boot, 0600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(d, "out")
	if err := run(key, sysPath, bootPath, "a", "1.2.3", 5, out, false); err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(filepath.Join(out, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	sig, err := os.ReadFile(filepath.Join(out, "manifest.sig"))
	if err != nil {
		t.Fatal(err)
	}
	if !ed25519.Verify(pub, payload, sig) {
		t.Fatal("invalid signature")
	}
	var m manifest
	if err := json.Unmarshal(payload, &m); err != nil || m.Board != "cube" || m.Slot != "a" || m.Sequence != 5 {
		t.Fatal("invalid manifest")
	}
	if err := run(key, sysPath, bootPath, "a", "1.2.3", 5, out, false); err == nil {
		t.Fatal("overwrote existing output")
	}
	if err := run(key, sysPath, bootPath, "b", "1.2.3", 5, filepath.Join(d, "wrong"), false); err == nil {
		t.Fatal("accepted wrong boot target")
	}
	copy(system[1200:], faultMarker)
	if err := os.WriteFile(sysPath, system, 0600); err != nil {
		t.Fatal(err)
	}
	if err := run(key, sysPath, bootPath, "a", "1.2.3", 5, filepath.Join(d, "fault-denied"), false); err == nil {
		t.Fatal("signed a fault image without explicit opt-in")
	}
	if err := run(key, sysPath, bootPath, "a", "1.2.3", 5, filepath.Join(d, "fault-test"), true); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(d, "repo")
	if err := os.Mkdir(repo, 0700); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("git", "init", "-q", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, output)
	}
	inRepo := filepath.Join(repo, "key.pem")
	if err := os.WriteFile(inRepo, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readKey(inRepo); err == nil {
		t.Fatal("accepted a key inside a worktree")
	}
}
