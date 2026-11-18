//go:build cube_slot

package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
)

func TestSignedCubeManifest(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("fixture"))
	m := cubeUpdateManifest{Format: 1, Board: "cube", Version: "1.2.3", Sequence: 2, Slot: "a", SystemSize: 512, SystemSHA256: hex.EncodeToString(sum[:]), BootSize: 128, BootSHA256: hex.EncodeToString(sum[:])}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	sig := ed25519.Sign(priv, data)
	if got, err := verifyCubeManifest(pub, data, sig); err != nil || got != m {
		t.Fatalf("valid manifest: %v", err)
	}
	changed := append([]byte(nil), data...)
	changed[10] ^= 1
	if _, err := verifyCubeManifest(pub, changed, sig); err == nil {
		t.Fatal("accepted modified manifest")
	}
	if _, err := verifyCubeManifest(pub, append(data, ' '), ed25519.Sign(priv, append(data, ' '))); err == nil {
		t.Fatal("accepted noncanonical manifest")
	}
	m.Board = "other"
	data, _ = json.Marshal(m)
	if _, err := verifyCubeManifest(pub, data, ed25519.Sign(priv, data)); err == nil {
		t.Fatal("accepted different board")
	}
	m.Board, m.Slot = "cube", "../a"
	data, _ = json.Marshal(m)
	if _, err := verifyCubeManifest(pub, data, ed25519.Sign(priv, data)); err == nil {
		t.Fatal("accepted invalid slot")
	}
}
