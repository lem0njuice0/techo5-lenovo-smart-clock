//go:build cube_slot

package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCubePendingRequiresSignatureAndDoesNotOverwrite(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256([]byte("fixture"))
	m := cubeUpdateManifest{Format: 1, Board: "cube", Version: "1.2.3", Sequence: 2, Slot: "a", SystemSize: 2048, SystemSHA256: hex.EncodeToString(h[:]), BootSize: 6144, BootSHA256: hex.EncodeToString(h[:])}
	payload, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	sig := ed25519.Sign(priv, payload)
	d := t.TempDir()
	path := filepath.Join(d, "pending")
	bad := append([]byte(nil), sig...)
	bad[0] ^= 1
	if err := writeCubePending(path, pub, payload, bad); err == nil {
		t.Fatal("accepted invalid signature")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("invalid signature created pending")
	}
	if err := writeCubePending(path, pub, payload, sig); err != nil {
		t.Fatal(err)
	}
	pending, err := readCubePending(path, pub); if err != nil {
		t.Fatal(err)
	}
	if err := writeCubePending(path, pub, payload, sig); err == nil {
		t.Fatal("overwrote existing pending")
	}
	changed := cubePending{Manifest:append([]byte(nil),pending.Manifest...),Signature:pending.Signature}; changed.Manifest[0] ^= 1
	if err := removeCubePending(path,pub,changed); err == nil { t.Fatal("removed a changed pending record") }
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := readCubePending(path, pub); err == nil {
		t.Fatal("accepted public pending file")
	}
	link := filepath.Join(d, "link")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := readCubePending(link, pub); err == nil {
		t.Fatal("followed pending symlink")
	}
	if err := os.Chmod(path,0600); err != nil { t.Fatal(err) }
	if err := removeCubePending(path,pub,pending); err != nil { t.Fatal(err) }
	if _,err := os.Lstat(path); !os.IsNotExist(err) { t.Fatal("pending record remains after confirmation") }
}
